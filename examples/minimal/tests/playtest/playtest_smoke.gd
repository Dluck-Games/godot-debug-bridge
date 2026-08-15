extends DebugBridgePlaytestSuite


func suite_name() -> String:
	return "smoke"


func scenario_config() -> Dictionary:
	return {"name": "playtest-smoke", "value": 7}


func setup_checkpoints() -> void:
	register_checkpoint("scenario crossed the host boundary")


func check_next_checkpoint(world: Variant) -> bool:
	return world is Node and world.get_meta("scenario").get("value") == 7
