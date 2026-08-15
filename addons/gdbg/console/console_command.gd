# CommandSpec / SubcommandSpec / ParamSpec — immutable declarative command
# descriptors consumed by ConsoleRegistry. Modules build these from their
# build() method; the registry never inspects handlers by reflection.
class_name ConsoleCommandSpec
extends RefCounted


class ParamSpec extends RefCounted:
	var name: String = ""
	var type: int = 0
	var is_required: bool = false
	var default: Variant = null
	var enum_values: Array = []

	static func required(p_name: String, p_type: int) -> ParamSpec:
		var p := ParamSpec.new()
		p.name = p_name
		p.type = p_type
		p.is_required = true
		return p

	static func optional(p_name: String, p_type: int, p_default: Variant) -> ParamSpec:
		var p := ParamSpec.new()
		p.name = p_name
		p.type = p_type
		p.is_required = false
		p.default = p_default
		return p

	static func enum_required(p_name: String, values: Array) -> ParamSpec:
		var p := ParamSpec.new()
		p.name = p_name
		p.type = 4  # ConsoleParamType.ENUM
		p.is_required = true
		p.enum_values = values
		return p


class SubcommandSpec extends RefCounted:
	var name: String = ""
	var desc: String = ""
	var params: Array = []  # Array[ParamSpec]
	var handler: Callable = Callable()

	func _init(p_name: String = "", p_desc: String = "", p_params: Array = [], p_handler: Callable = Callable()) -> void:
		name = p_name
		desc = p_desc
		params = p_params
		handler = p_handler


class CommandSpec extends RefCounted:
	var name: String = ""
	var desc: String = ""
	var subcommands: Array = []     # Array[SubcommandSpec]; empty → flat
	var flat_params: Array = []     # Array[ParamSpec]; only when subcommands empty
	var flat_handler: Callable = Callable()
	var default_subcommand: String = ""

	static func category(p_name: String, p_desc: String, p_subs: Array, p_default: String = "") -> CommandSpec:
		var c := CommandSpec.new()
		c.name = p_name
		c.desc = p_desc
		c.subcommands = p_subs
		c.default_subcommand = p_default
		return c

	static func flat(p_name: String, p_desc: String, p_params: Array, p_handler: Callable) -> CommandSpec:
		var c := CommandSpec.new()
		c.name = p_name
		c.desc = p_desc
		c.flat_params = p_params
		c.flat_handler = p_handler
		return c

	func is_flat() -> bool:
		return subcommands.is_empty()

	func find_subcommand(sub_name: String) -> SubcommandSpec:
		for s in subcommands:
			if (s as SubcommandSpec).name == sub_name:
				return s
		return null
