class_name DebugBridgeTestHostBase
extends Node

## Game-owned adapter contract for GDBG integration and playtest execution.
##
## GDBG owns orchestration, reports, checkpoints and recording. A project owns
## every game concept: scenario data, fixtures, world creation and lifecycle.
## The boundary intentionally uses only Variant, Dictionary, Array and Callable
## so the public addon never imports project scripts or assumes a particular
## world model, content schema, generator, or scene-package format.

const HOST_NODE_PATH := "/root/DebugBridgeTestHost"


static func instance() -> Variant:
	var tree := Engine.get_main_loop() as SceneTree
	if tree == null:
		return null
	var node := tree.root.get_node_or_null(HOST_NODE_PATH)
	if node == null or not (node is DebugBridgeTestHostBase):
		return null
	return node


## Optional data-driven fixture service for project suite helpers. `kind` and
## `data` are defined entirely by the project adapter.
func build_fixture(kind: String, data: Variant = {}) -> Variant:
	return _not_configured("build_fixture:%s" % kind)


## Build the opaque scenario object consumed by the project lifecycle. GDBG
## stores and forwards the value without inspecting its fields.
func build_scenario(config: Dictionary = {}) -> Variant:
	return _not_configured("build_scenario")


## Human-readable label used only in runner output.
func scenario_label(_scenario: Variant) -> String:
	return "scenario"


# ---------------------------------------------------------------------------
# Integration lifecycle
# ---------------------------------------------------------------------------

func integration_setup() -> Variant:
	return {}


## Load an opaque scenario. Return an empty string/Dictionary on success or a
## Dictionary containing `error` on failure. Project-specific overrides belong
## in `options`; GDBG does not assign meaning to those keys.
func integration_load(
	scenario: Variant,
	after_loaded_hook: Callable = Callable(),
	options: Dictionary = {}
) -> Variant:
	return _not_configured("integration_load")


func integration_world() -> Variant:
	return _not_configured("integration_world")


func integration_teardown() -> void:
	pass


func integration_quit(exit_code: int = 0) -> void:
	var tree := get_tree()
	if tree != null:
		tree.quit(exit_code)


# ---------------------------------------------------------------------------
# Playtest production lifecycle
# ---------------------------------------------------------------------------

func playtest_boot(_launch_args: Dictionary = {}) -> Variant:
	return {}


func playtest_load(
	scenario: Variant,
	after_loaded_hook: Callable = Callable(),
	options: Dictionary = {}
) -> Variant:
	return _not_configured("playtest_load")


func playtest_world() -> Variant:
	return _not_configured("playtest_world")


## Run the suite against the project world and return 0 (passed), 1 (failed),
## or 2 (error). Projects may delegate back to suite.test_run/run_default.
func playtest_run(_suite: Variant, _world: Variant) -> int:
	_not_configured("playtest_run")
	return 2


func playtest_quit(exit_code: int = 0) -> void:
	var tree := get_tree()
	if tree != null:
		tree.quit(exit_code)


func _not_configured(method: String) -> Dictionary:
	var message := "DebugBridgeTestHost: '%s' is not configured. Extend DebugBridgeTestHostBase and register the subclass as the '%s' autoload." % [method, HOST_NODE_PATH]
	push_error(message)
	return {"error": message}
