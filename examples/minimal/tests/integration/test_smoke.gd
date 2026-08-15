extends DebugBridgeIntegrationTestSuite


func scenario_config() -> Dictionary:
	return {"name": "integration-smoke", "value": 42}


func test_run(world: Variant) -> Variant:
	var result := DebugBridgeTestResult.new()
	result.assert_true(world is Node, "host supplied a world node")
	result.assert_equal(world.get_meta("scenario").get("value"), 42, "scenario crossed the host boundary")
	return result
