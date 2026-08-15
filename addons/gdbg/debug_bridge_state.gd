class_name DebugBridgeState
extends RefCounted

## Shared state-root resolver. The configured environment variable wins, then
## $XDG_STATE_HOME/app_name, then ~/.local/state/app_name. IPC and artifacts
## always use absolute paths. Projects may override the two gdbg/state settings.

const DEBUG_SUBDIR := "debug"
const IPC_SUBDIR := "ipc"
const SCREENSHOTS_SUBDIR := "screenshots"

static var _env_var: String = ""
static var _app_name: String = ""


static func _setting_env_var() -> String:
	if _env_var.is_empty():
		_env_var = str(ProjectSettings.get_setting("gdbg/state/env_var", "GDBG_STATE"))
	return _env_var


static func _setting_app_name() -> String:
	if _app_name.is_empty():
		_app_name = str(ProjectSettings.get_setting("gdbg/state/app_name", "gdbg"))
	return _app_name


## Absolute GDBG state root.
static func state_root() -> String:
	var injected := OS.get_environment(_setting_env_var())
	if not injected.is_empty():
		return _strip_trailing_slash(injected)
	var xdg := OS.get_environment("XDG_STATE_HOME")
	if not xdg.is_empty():
		return _strip_trailing_slash(xdg).path_join(_setting_app_name())
	var home := OS.get_environment("HOME")
	if home.is_empty():
		home = OS.get_environment("USERPROFILE")
	return _strip_trailing_slash(home).path_join(".local").path_join("state").path_join(_setting_app_name())


static func debug_dir() -> String:
	return state_root().path_join(DEBUG_SUBDIR)


## File IPC directory containing command, result and temp_script.gd.
static func ipc_dir() -> String:
	return debug_dir().path_join(IPC_SUBDIR)


## Final screenshot and piggyback-capture directory.
static func screenshots_dir() -> String:
	return debug_dir().path_join(SCREENSHOTS_SUBDIR)


static func command_file() -> String:
	return ipc_dir().path_join(DebugBridgeProtocol.COMMAND_FILE_NAME)


static func result_file() -> String:
	return ipc_dir().path_join(DebugBridgeProtocol.RESULT_FILE_NAME)


static func temp_script_file() -> String:
	return ipc_dir().path_join(DebugBridgeProtocol.TEMP_SCRIPT_FILE_NAME)


## Ensure a directory and its parents exist.
static func ensure_dir(path: String) -> bool:
	if DirAccess.dir_exists_absolute(path):
		return true
	return DirAccess.make_dir_recursive_absolute(path) == OK


## Ensure the core IPC and screenshot directories exist.
static func ensure_debug_dirs() -> void:
	ensure_dir(ipc_dir())
	ensure_dir(screenshots_dir())


static func _strip_trailing_slash(path: String) -> String:
	return path.rstrip("/")
