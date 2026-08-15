extends Node


func _ready() -> void:
	var test_runtime: Variant = DebugBridgeTestRuntime.instance()
	if test_runtime != null and test_runtime.is_playtest_launch():
		call_deferred("_run_playtest", test_runtime)
		return
	if not "--gdbg-smoke" in OS.get_cmdline_user_args():
		print("GDBG minimal example is running")
		return
	var host := get_node_or_null("/root/DebugBridgeHost")
	var failures: Array[String] = []
	if host == null:
		failures.append("DebugBridgeHost autoload is missing")
	else:
		if host.execute("ping") != "pong":
			failures.append("ping command did not return pong")
		if host.execute("echo hello from gdbg") != "hello from gdbg":
			failures.append("echo command did not preserve its expression")
		var help_text: String = host.execute("help")
		if help_text.find("ping") < 0 or help_text.find("echo") < 0:
			failures.append("built-in help did not list project commands")
		if host.execute("pnig").find("Did you mean 'ping'?") < 0:
			failures.append("unknown-command suggestion did not find ping")
	if get_node_or_null("/root/AIDebugBridge") == null:
		failures.append("AIDebugBridge autoload is missing")
	if DebugBridgeProtocol.VERSION != 1:
		failures.append("unexpected protocol version")
	if failures.is_empty():
		print("GDBG_SMOKE_OK")
		get_tree().quit(0)
		return
	for failure in failures:
		push_error("GDBG smoke: %s" % failure)
	get_tree().quit(1)


func _run_playtest(test_runtime: Variant) -> void:
	var suite: Variant = test_runtime.active_suite()
	if suite == null:
		push_error("GDBG minimal playtest: %s" % test_runtime.load_error())
		test_runtime.production_quit(2)
		return
	var setup_result: Variant = test_runtime.production_setup()
	if setup_result is Dictionary and (setup_result as Dictionary).has("error"):
		push_error(str((setup_result as Dictionary).get("error")))
		test_runtime.production_quit(2)
		return
	var load_result: Variant = test_runtime.production_load_scenario(suite.after_loaded_hook)
	if load_result is Dictionary and (load_result as Dictionary).has("error"):
		push_error(str((load_result as Dictionary).get("error")))
		test_runtime.production_quit(2)
		return
	var exit_code: int = await test_runtime.production_run(test_runtime.production_world())
	test_runtime.production_quit(exit_code)
