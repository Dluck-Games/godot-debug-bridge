# ConsoleCompleter — maps (ParamType, prefix) → Array[String] of candidates.
# Stateless; all methods static. Game-agnostic core: BOOL_TOGGLE / ENUM
# completion plus prefix filtering. Game-specific param types are delegated
# to the optional /root/DebugBridgeHost completion hook when present.
class_name ConsoleCompleter
extends RefCounted

const ParamTypeScript := preload("res://addons/gdbg/console/console_types.gd")


static func for_param_type(param_type: int, prefix: String, enum_values: Array) -> Array[String]:
	match param_type:
		ParamTypeScript.BOOL_TOGGLE:
			return _filter_prefix(["off", "on"], prefix)
		ParamTypeScript.ENUM:
			var sorted := enum_values.duplicate()
			sorted.sort()
			var typed: Array[String] = []
			for v in sorted:
				typed.append(String(v))
			return _filter_prefix(typed, prefix)
		_:
			return _from_host_hook(param_type, prefix, enum_values)


static func _from_host_hook(param_type: int, prefix: String, enum_values: Array) -> Array[String]:
	var main_loop := Engine.get_main_loop()
	if main_loop == null:
		return [] as Array[String]
	var host: Node = main_loop.root.get_node_or_null("/root/DebugBridgeHost")
	if host == null or not host.has_method("complete_param_type"):
		return [] as Array[String]
	var typed_enum: Array[String] = []
	for v in enum_values:
		typed_enum.append(String(v))
	return host.complete_param_type(param_type, prefix, typed_enum)


static func _filter_prefix(sorted_source: Array, prefix: String) -> Array[String]:
	var results: Array[String] = []
	var lower := prefix.to_lower()
	for item in sorted_source:
		var s := String(item)
		if lower.is_empty() or s.to_lower().begins_with(lower):
			results.append(s)
	return results
