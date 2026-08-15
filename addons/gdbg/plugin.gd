@tool
extends EditorPlugin

## Registers GDBG defaults and the stable runtime autoloads. Project-owned
## DebugBridgeHost and DebugBridgeTestHost adapters are never registered here.

const ADDON_SCRIPT_SCREENSHOT_MANAGER := "res://addons/gdbg/screenshot_manager.gd"
const ADDON_SCRIPT_AI_DEBUG_BRIDGE := "res://addons/gdbg/ai_debug_bridge.gd"
const ADDON_SCRIPT_TEST_RUNTIME := "res://addons/gdbg/testing/test_runtime.gd"

const AUTOLOAD_SCREENSHOT_MANAGER := "ScreenshotManager"
const AUTOLOAD_AI_DEBUG_BRIDGE := "AIDebugBridge"
const AUTOLOAD_TEST_RUNTIME := "DebugBridgeTestRuntime"

const SETTING_STATE_ENV_VAR := "gdbg/state/env_var"
const SETTING_STATE_APP_NAME := "gdbg/state/app_name"
const SETTING_SUITE_PATTERN := "gdbg/playtest/suite_pattern"
const DEFAULT_SUITE_PATTERN := "res://tests/playtest/playtest_%s.gd"


func _enter_tree() -> void:
	_register_default_settings()
	add_autoload_singleton(AUTOLOAD_SCREENSHOT_MANAGER, ADDON_SCRIPT_SCREENSHOT_MANAGER)
	add_autoload_singleton(AUTOLOAD_AI_DEBUG_BRIDGE, ADDON_SCRIPT_AI_DEBUG_BRIDGE)
	add_autoload_singleton(AUTOLOAD_TEST_RUNTIME, ADDON_SCRIPT_TEST_RUNTIME)


func _exit_tree() -> void:
	# Editor shutdown also invokes _exit_tree. Runtime autoloads must remain in
	# project.godot so CLI/headless launches keep working after the editor closes.
	pass


func _disable_plugin() -> void:
	# Explicit plugin disable is the uninstall boundary; unlike editor shutdown,
	# it should remove the three GDBG-owned autoload entries.
	remove_autoload_singleton(AUTOLOAD_SCREENSHOT_MANAGER)
	remove_autoload_singleton(AUTOLOAD_AI_DEBUG_BRIDGE)
	remove_autoload_singleton(AUTOLOAD_TEST_RUNTIME)


func _register_default_settings() -> void:
	if not ProjectSettings.has_setting(SETTING_STATE_ENV_VAR):
		ProjectSettings.set_setting(SETTING_STATE_ENV_VAR, "GDBG_STATE")
	if not ProjectSettings.has_setting(SETTING_STATE_APP_NAME):
		ProjectSettings.set_setting(SETTING_STATE_APP_NAME, "gdbg")
	if not ProjectSettings.has_setting(SETTING_SUITE_PATTERN):
		ProjectSettings.set_setting(SETTING_SUITE_PATTERN, DEFAULT_SUITE_PATTERN)
