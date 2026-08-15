extends Node

func _ready() -> void:
	if not OS.get_cmdline_user_args().has("--gdbg-scene-smoke"):
		push_error("scene smoke requires the --gdbg-scene-smoke user argument; got: %s" % OS.get_cmdline_user_args())
		get_tree().quit(2)
		return
	print("GDBG_SCENE_TEST_OK")
	get_tree().quit(0)
