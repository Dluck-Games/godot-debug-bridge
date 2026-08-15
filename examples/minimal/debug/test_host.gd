extends DebugBridgeTestHostBase

var _world: Node = null


func build_fixture(_kind: String, data: Variant = {}) -> Variant:
	return data


func build_scenario(config: Dictionary = {}) -> Variant:
	return config.duplicate(true)


func scenario_label(scenario: Variant) -> String:
	if scenario is Dictionary:
		return String((scenario as Dictionary).get("name", "minimal"))
	return "minimal"


func integration_load(scenario: Variant, after_loaded_hook: Callable = Callable(), _options: Dictionary = {}) -> Variant:
	return _load_world(scenario, after_loaded_hook)


func integration_world() -> Variant:
	return _world


func integration_teardown() -> void:
	_free_world()


func playtest_load(scenario: Variant, after_loaded_hook: Callable = Callable(), _options: Dictionary = {}) -> Variant:
	return _load_world(scenario, after_loaded_hook)


func playtest_world() -> Variant:
	return _world


func playtest_run(suite: Variant, world: Variant) -> int:
	var result: Variant = null
	if suite.has_method("test_run"):
		result = await suite.test_run(world)
	elif suite.has_method("run_default"):
		result = await suite.run_default(world)
	if result is DebugBridgeTestResult:
		(result as DebugBridgeTestResult).print_report()
		return (result as DebugBridgeTestResult).exit_code()
	return 2


func _load_world(scenario: Variant, after_loaded_hook: Callable) -> Variant:
	_free_world()
	_world = Node.new()
	_world.name = "MinimalTestWorld"
	_world.set_meta("scenario", scenario)
	get_tree().root.add_child(_world)
	if after_loaded_hook.is_valid():
		after_loaded_hook.call(_world)
	return {}


func _free_world() -> void:
	if _world == null or not is_instance_valid(_world):
		_world = null
		return
	_world.queue_free()
	_world = null
