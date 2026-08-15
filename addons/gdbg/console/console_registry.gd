# ConsoleRegistry — aggregates command modules and dispatches execute /
# completion / help queries. Created per Service_Console instance.
# Game-agnostic: command module scripts are supplied by the caller in _init.
extends RefCounted

const ConsoleParserScript := preload("res://addons/gdbg/console/console_parser.gd")
const ConsoleCompleterScript := preload("res://addons/gdbg/console/console_completer.gd")
const ConsoleContextScript := preload("res://addons/gdbg/console/console_context.gd")
const CommandSpecScript := preload("res://addons/gdbg/console/console_command.gd")
const ParamTypeScript := preload("res://addons/gdbg/console/console_types.gd")

var _commands: Dictionary = {}  # String → CommandSpec
var _sorted_names: Array[String] = []
var _service: Object  # Service_Console back-ref
# MUST retain module instances for the registry's lifetime. SubcommandSpec /
# CommandSpec store Callable(module, "_handler"), which holds a weak reference
# to the module. If the module is freed, the Callable silently becomes a
# no-op (no error, no log). See res://addons/gdbg/console/console_command_module.gd
# for the wider contract.
var _modules: Array = []
# Module scripts accepted in _init; retained so setup() can be re-run after
# teardown(), mirroring the original const module list.
var _module_scripts: Array = []
# Context construction strategy used when invoking handlers: either a context
# GDScript (instantiated with the console service) or a Callable(service)
# returning a context instance. Defaults to the addon's generic ConsoleContext.
var _context_factory: Variant


func _init(service: Object, module_scripts: Array = [], context_factory: Variant = null) -> void:
	_service = service
	_module_scripts = module_scripts
	_context_factory = context_factory if context_factory != null else ConsoleContextScript
	setup()


func setup() -> void:
	_commands.clear()
	_modules.clear()
	_register_builtin_help()
	for module_script in _module_scripts:
		var module = module_script.new()
		_modules.append(module)
		var specs: Array = module.build()
		for spec in specs:
			_register_spec(spec)
	_sorted_names.clear()
	for n in _commands.keys():
		_sorted_names.append(String(n))
	_sorted_names.sort()


func _register_builtin_help() -> void:
	var help_spec = CommandSpecScript.CommandSpec.flat(
		"help",
		"Show available commands or details for one command",
		[CommandSpecScript.ParamSpec.optional("command", ParamTypeScript.STRING, "")],
		Callable(self, "_execute_help")
	)
	_register_spec(help_spec)


func _execute_help(_context: Object, args: Dictionary) -> String:
	return help_for(String(args.get("command", "")))


func teardown() -> void:
	_commands.clear()
	_sorted_names.clear()
	_modules.clear()


# Exposed for unit tests that bypass module discovery.
func _register_spec(spec) -> void:
	var cmd_name := String(spec.name)
	assert(cmd_name == cmd_name.to_lower(), "CommandSpec name must be lowercase: '%s'" % cmd_name)
	if not spec.is_flat():
		for sub in spec.subcommands:
			var sub_name := String(sub.name)
			assert(sub_name == sub_name.to_lower(), "SubcommandSpec name must be lowercase: '%s %s'" % [cmd_name, sub_name])
	_commands[cmd_name] = spec


# --- Public API used by Service_Console ---

func command_names() -> Array[String]:
	return _sorted_names.duplicate()


func has_command(name: String) -> bool:
	return _commands.has(name)


func execute(input: String) -> String:
	var parsed: Dictionary = ConsoleParserScript.parse(input)
	var cmd_name: String = String(parsed.cmd).to_lower()
	if cmd_name.is_empty():
		return ""

	if not _commands.has(cmd_name):
		return _unknown_cmd_message(cmd_name)

	var spec = _commands[cmd_name]
	if spec.is_flat():
		return _invoke_flat(spec, parsed.subcmd_candidate, parsed.positionals)
	return _invoke_category(spec, parsed.subcmd_candidate, parsed.positionals)


func describe_cursor(input: String, cursor: int) -> Dictionary:
	return ConsoleParserScript.describe_cursor(input, cursor)


func get_completions_at(state: Dictionary) -> Array[String]:
	match state.get("slot", ""):
		"cmd":
			return _cmd_completions(state.get("prefix", ""))
		"subcmd":
			return _subcmd_completions(state.get("cmd", ""), state.get("prefix", ""))
		"positional":
			return _positional_completions(state)
		_:
			return [] as Array[String]


func help_for(name: String) -> String:
	if name.is_empty():
		var lines: Array[String] = ["Available commands:"]
		for n in _sorted_names:
			var spec = _commands[n]
			if spec.desc.is_empty():
				lines.append("  " + n)
			else:
				lines.append("  %s - %s" % [n, spec.desc])
		return "\n".join(lines)

	if not _commands.has(name):
		return "Unknown command: " + name
	var spec = _commands[name]
	return _build_usage(spec)


# --- Resolution + dispatch ---

func _invoke_flat(spec, second_token: String, positionals: Array) -> String:
	var all_positionals: Array = []
	if not second_token.is_empty():
		all_positionals.append(second_token)
	all_positionals.append_array(positionals)
	return _bind_and_call(spec.flat_handler, spec.flat_params, all_positionals, _usage_prefix_flat(spec))


func _invoke_category(spec, subcmd_name: String, positionals: Array) -> String:
	if subcmd_name.is_empty():
		if not spec.default_subcommand.is_empty():
			var default_sub = spec.find_subcommand(spec.default_subcommand)
			if default_sub != null:
				return _bind_and_call(default_sub.handler, default_sub.params, positionals, _usage_prefix_sub(spec, default_sub))
		return _subcommand_list_message(spec)

	var sub = spec.find_subcommand(subcmd_name.to_lower())
	if sub == null:
		return _unknown_subcmd_message(spec, subcmd_name)

	return _bind_and_call(sub.handler, sub.params, positionals, _usage_prefix_sub(spec, sub))


func _bind_and_call(handler: Callable, param_specs: Array, positionals: Array, usage_prefix: String) -> String:
	var args := {}
	for i in range(param_specs.size()):
		var p = param_specs[i]
		if i < positionals.size():
			var token: String = String(positionals[i])
			# EXPRESSION slurps all remaining tokens.
			if p.type == ParamTypeScript.EXPRESSION:
				token = " ".join(positionals.slice(i))
			var coerced = _coerce(p, token)
			if coerced.has("error"):
				return coerced.error
			args[p.name] = coerced.value
		else:
			if p.is_required:
				return "Usage: %s%s" % [usage_prefix, _format_params(param_specs)]
			args[p.name] = p.default
	var ctx := _make_context()
	return handler.call(ctx, args)


func _make_context() -> Object:
	if _context_factory is Callable:
		var factory: Callable = _context_factory
		return factory.call(_service)
	if _context_factory != null:
		return _context_factory.new(_service)
	return ConsoleContextScript.new(_service)


func _coerce(param, token: String) -> Dictionary:
	match param.type:
		ParamTypeScript.INT:
			if not token.is_valid_int():
				return {"error": "Invalid %s: '%s' (expected int)" % [param.name, token]}
			return {"value": token.to_int()}
		ParamTypeScript.FLOAT:
			if not token.is_valid_float():
				return {"error": "Invalid %s: '%s' (expected float)" % [param.name, token]}
			return {"value": token.to_float()}
		ParamTypeScript.BOOL_TOGGLE:
			var low := token.to_lower()
			if low in ["on", "true", "1"]:
				return {"value": true}
			if low in ["off", "false", "0"]:
				return {"value": false}
			return {"error": "Invalid %s: '%s' (expected on/off)" % [param.name, token]}
		ParamTypeScript.ENUM:
			if not (token in param.enum_values):
				return {"error": "Invalid %s: '%s' (expected one of %s)" % [param.name, token, ", ".join(param.enum_values)]}
			return {"value": token}
		_:
			return {"value": token}


# --- Messages ---

func _unknown_cmd_message(cmd: String) -> String:
	var suggestion := _nearest_match(cmd, _sorted_names)
	if suggestion.is_empty():
		return "Unknown command: '%s'. Type 'help' for available commands." % cmd
	return "Unknown command: '%s'. Did you mean '%s'?" % [cmd, suggestion]


func _unknown_subcmd_message(spec, sub: String) -> String:
	var names: Array[String] = []
	for s in spec.subcommands:
		names.append(s.name)
	names.sort()
	var suggestion := _nearest_match(sub, names)
	var head := "Unknown subcommand: '%s %s'." % [spec.name, sub]
	if not suggestion.is_empty():
		return "%s Did you mean '%s %s'?" % [head, spec.name, suggestion]
	return "%s Available: %s" % [head, ", ".join(names)]


func _subcommand_list_message(spec) -> String:
	var names: Array[String] = []
	for s in spec.subcommands:
		names.append(s.name)
	names.sort()
	return "Usage: %s <%s>" % [spec.name, "|".join(names)]


func _usage_prefix_flat(spec) -> String:
	return spec.name + " "


func _usage_prefix_sub(spec, sub) -> String:
	return "%s %s " % [spec.name, sub.name]


func _format_params(param_specs: Array) -> String:
	var parts: Array[String] = []
	for p in param_specs:
		if p.is_required:
			parts.append("<%s>" % p.name)
		elif p.default == null:
			parts.append("[%s]" % p.name)
		else:
			parts.append("[%s=%s]" % [p.name, str(p.default)])
	return " ".join(parts)


func _build_usage(spec) -> String:
	if spec.is_flat():
		var line := "%s %s" % [spec.name, _format_params(spec.flat_params)]
		if spec.desc.is_empty():
			return line
		return "%s\n  %s" % [line, spec.desc]
	var lines: Array[String] = []
	for sub in spec.subcommands:
		var l := "%s %s %s" % [spec.name, sub.name, _format_params(sub.params)]
		if sub.desc.is_empty():
			lines.append(l)
		else:
			lines.append("%s\n  %s" % [l, sub.desc])
	return "\n".join(lines)


# --- Completion ---

func _cmd_completions(prefix: String) -> Array[String]:
	var lower := prefix.to_lower()
	var results: Array[String] = []
	for n in _sorted_names:
		if lower.is_empty() or n.begins_with(lower):
			results.append(n)
	return results


func _subcmd_completions(cmd: String, prefix: String) -> Array[String]:
	if not _commands.has(cmd):
		return [] as Array[String]
	var spec = _commands[cmd]
	if spec.is_flat():
		# For flat commands, second slot is param[0].
		return _param_completions(spec.flat_params, 0, prefix)
	var results: Array[String] = []
	var lower := prefix.to_lower()
	for s in spec.subcommands:
		if lower.is_empty() or s.name.begins_with(lower):
			results.append(s.name)
	results.sort()
	return results


func _positional_completions(state: Dictionary) -> Array[String]:
	var cmd_name := String(state.get("cmd", ""))
	if not _commands.has(cmd_name):
		return [] as Array[String]
	var spec = _commands[cmd_name]
	var prefix := String(state.get("prefix", ""))
	var param_index := int(state.get("param_index", 0))
	if spec.is_flat():
		# Flat: state.subcmd is really the first positional; param_index is offset by 1.
		return _param_completions(spec.flat_params, param_index + 1, prefix)
	var sub = spec.find_subcommand(String(state.get("subcmd", "")).to_lower())
	if sub == null:
		return [] as Array[String]
	return _param_completions(sub.params, param_index, prefix)


func _param_completions(param_specs: Array, index: int, prefix: String) -> Array[String]:
	if index < 0 or index >= param_specs.size():
		return [] as Array[String]
	var p = param_specs[index]
	return ConsoleCompleterScript.for_param_type(p.type, prefix, p.enum_values)


# --- Typo hints (prefix match) ---

func _nearest_match(target: String, candidates: Array[String]) -> String:
	var best := ""
	var best_score := -1
	for c in candidates:
		var score := 0
		var min_len := mini(target.length(), c.length())
		for i in range(min_len):
			if target[i] == c[i]:
				score += 1
			else:
				break
		if score > best_score:
			best_score = score
			best = c
	if best_score <= 0:
		return ""
	return best
