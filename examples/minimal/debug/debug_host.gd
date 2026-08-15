extends DebugBridgeConsoleHostBase

const MinimalCommands := preload("res://debug/minimal_commands.gd")


func command_modules() -> Array:
	return [MinimalCommands]
