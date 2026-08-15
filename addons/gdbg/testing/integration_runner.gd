extends Node

## Generic GDBG integration-test entry point.
##
## The runner resolves a suite, validates the generic suite capability, then
## delegates setup/load/world/teardown/quit to DebugBridgeTestHostBase. The
## scenario is opaque and project-specific options are forwarded unchanged.

var _suite: DebugBridgeAutomationTestSuite = null
var _host: Variant = null
var _no_exit: bool = false


func _ready() -> void:
	await get_tree().process_frame

	var suite_path := _parse_suite_path()
	if suite_path.is_empty():
		await _fail("Missing --integration= argument")
		return
	if not ResourceLoader.exists(suite_path):
		await _fail("Integration suite script not found: %s" % suite_path)
		return

	var script := load(suite_path) as GDScript
	if script == null or not script.can_instantiate():
		await _fail("Failed to load integration suite script: %s" % suite_path)
		return

	var instance = script.new()
	if not (instance is DebugBridgeAutomationTestSuite):
		_release_invalid_instance(instance)
		await _fail("Integration suite must extend DebugBridgeAutomationTestSuite: %s" % suite_path)
		return
	_suite = instance as DebugBridgeAutomationTestSuite
	if not _suite.is_integration_suite():
		await _fail("Integration suite did not opt into the integration contract: %s" % suite_path)
		return
	if _suite.scenario == null or _is_error(_suite.scenario):
		await _fail("Integration suite has no valid scenario: %s" % suite_path)
		return

	_host = DebugBridgeTestHostBase.instance()
	if _host == null:
		await _fail("DebugBridgeTestHost is not registered")
		return

	var setup_result: Variant = _host.integration_setup()
	if _is_error(setup_result):
		await _fail(_error_text(setup_result))
		return

	print("[integration] Loaded suite: %s (scenario: %s)" % [suite_path, _suite.scenario_label()])
	var load_result: Variant = _host.integration_load(
		_suite.scenario,
		_suite.after_loaded_hook if _suite.after_loaded_hook.is_valid() else Callable(),
		_suite.integration_options()
	)
	if _is_error(load_result):
		await _fail(_error_text(load_result))
		return

	await get_tree().process_frame
	await get_tree().process_frame

	var world: Variant = _host.integration_world()
	var test_result: Variant = null
	if _suite.has_method("test_run"):
		test_result = await _suite.test_run(world)
	if test_result == null:
		print("[integration] No test_run defined, scenario loaded successfully")
		if not _no_exit:
			await _host.integration_quit(0)
		return
	if test_result is DebugBridgeTestResult:
		var result := test_result as DebugBridgeTestResult
		result.print_report()
		if not _no_exit:
			await _host.integration_quit(result.exit_code())
		return

	print("[integration] test_run returned a non-DebugBridgeTestResult value")
	if not _no_exit:
		await _host.integration_quit(0)


func _exit_tree() -> void:
	if _host != null:
		_host.integration_teardown()


func _parse_suite_path() -> String:
	var suite_path := ""
	for arg: String in OS.get_cmdline_user_args():
		if arg.begins_with("--integration="):
			suite_path = arg.substr("--integration=".length())
		elif arg == "--no-exit":
			_no_exit = true
	return suite_path


func _fail(message: String) -> void:
	push_error("[integration] %s" % message)
	print("[FAIL] %s" % message)
	if _no_exit:
		return
	if _host != null:
		await _host.integration_quit(1)
	else:
		get_tree().quit(1)


func _is_error(value: Variant) -> bool:
	return value is Dictionary and (value as Dictionary).has("error")


func _error_text(value: Variant) -> String:
	if value is Dictionary:
		return String((value as Dictionary).get("error", "unknown error"))
	return str(value)


func _release_invalid_instance(instance: Variant) -> void:
	if instance is RefCounted:
		instance = null
	elif instance is Object:
		(instance as Object).free()
