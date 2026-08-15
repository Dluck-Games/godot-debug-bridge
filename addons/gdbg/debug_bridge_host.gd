class_name DebugBridgeHostBase
extends Node

## Game-side host contract. AIDebugBridge calls command execution and input
## injection through /root/DebugBridgeHost. The safe defaults never import a
## particular game. Host projects register a DebugBridgeHost autoload that
## extends this class or DebugBridgeConsoleHostBase.


## Execute one console command and return its text result.
func execute(command: String) -> String:
	push_warning("DebugBridgeHost: host not configured, ignoring command: %s" % command)
	return "error: DebugBridgeHost not configured"


## Inject an input action press.
func simulate_action_pressed(action: String) -> void:
	push_warning("DebugBridgeHost: host not configured, ignoring action press: %s" % action)


## Inject an input action release.
func simulate_action_released(action: String) -> void:
	push_warning("DebugBridgeHost: host not configured, ignoring action release: %s" % action)


## Optional completion hook for project-defined parameter types.
func complete_param_type(param_type: int, partial: String, enum_values: Array[String]) -> Array[String]:
	return [] as Array[String]
