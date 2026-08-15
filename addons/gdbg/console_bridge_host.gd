class_name DebugBridgeConsoleHostBase
extends "res://addons/gdbg/debug_bridge_host.gd"

## Ready-to-extend host for projects that use the GDBG console framework.
## A game supplies command module scripts and, optionally, a context factory.
## The registry owns module instances and dispatches commands without the addon
## importing project code. Input injection and custom parameter completion
## remain regular DebugBridgeHostBase hooks.

const ConsoleRegistryScript := preload("res://addons/gdbg/console/console_registry.gd")

var _console_registry: Variant = null


## Return scripts extending ConsoleCommandModule. Each build() method returns
## declarative ConsoleCommandSpec values.
func command_modules() -> Array:
	return []


## Return a GDScript or Callable(service) used to construct handler contexts.
## Null selects the addon's minimal ConsoleContext.
func console_context_factory() -> Variant:
	return null


## Object exposed as context.service by the default ConsoleContext.
func console_service() -> Object:
	return self


func execute(command: String) -> String:
	return str(_ensure_console_registry().execute(command))


func command_names() -> Array[String]:
	return _ensure_console_registry().command_names()


func help_text(command_name: String = "") -> String:
	return _ensure_console_registry().help_for(command_name)


func describe_cursor(input: String, cursor: int) -> Dictionary:
	return _ensure_console_registry().describe_cursor(input, cursor)


func get_completions_at(state: Dictionary) -> Array[String]:
	return _ensure_console_registry().get_completions_at(state)


func reset_console_registry() -> void:
	if _console_registry != null:
		_console_registry.teardown()
	_console_registry = null


func _exit_tree() -> void:
	reset_console_registry()


func _ensure_console_registry() -> Variant:
	if _console_registry == null:
		_console_registry = ConsoleRegistryScript.new(
			console_service(),
			command_modules(),
			console_context_factory()
		)
	return _console_registry
