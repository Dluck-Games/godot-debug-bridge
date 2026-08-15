class_name DebugBridgeAutomationTestSuite
extends RefCounted

## Generic base for addon-driven integration and playtest suites.
##
## `scenario` is opaque to GDBG. Projects may construct it directly or provide
## a DebugBridgeTestHostBase.build_scenario implementation and override
## scenario_config(). Game-specific convenience factories belong in a project
## subclass and can delegate through make_fixture().

var scenario: Variant = null
var after_loaded_hook: Callable = Callable()


func _init(p_scenario: Variant = null) -> void:
	scenario = p_scenario if p_scenario != null else make_scenario(scenario_config())


func scenario_config() -> Dictionary:
	return {}


static func make_scenario(config: Dictionary = {}) -> Variant:
	var host := _host()
	if host == null:
		return _host_missing("build_scenario")
	return host.build_scenario(config)


static func make_fixture(kind: String, data: Variant = {}) -> Variant:
	var host := _host()
	if host == null:
		return _host_missing("build_fixture:%s" % kind)
	return host.build_fixture(kind, data)


func scenario_label() -> String:
	var host := _host()
	if host == null:
		return "scenario"
	return host.scenario_label(scenario)


func integration_options() -> Dictionary:
	return {}


func playtest_options() -> Dictionary:
	return {}


func is_integration_suite() -> bool:
	return false


func _wait_frames(world: Variant, count: int) -> void:
	if world == null or not world.has_method("get_tree"):
		return
	for _i in range(maxi(count, 0)):
		await world.get_tree().process_frame


func _wait_until(world: Variant, condition: Callable, timeout_frames: int = 3000) -> bool:
	if not condition.is_valid():
		return false
	for _i in range(maxi(timeout_frames, 0)):
		if world != null and world.has_method("get_tree"):
			await world.get_tree().process_frame
		if bool(condition.call()):
			return true
	return bool(condition.call())


static func _host() -> Variant:
	return DebugBridgeTestHostBase.instance()


static func _host_missing(method: String) -> Dictionary:
	var message := "DebugBridgeAutomationTestSuite: '%s' requires a DebugBridgeTestHostBase autoload at /root/DebugBridgeTestHost." % method
	push_error(message)
	return {"error": message}
