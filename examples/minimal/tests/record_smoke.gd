extends Node


func run(rec: DebugBridgeRecordSession) -> void:
	var layer := CanvasLayer.new()
	layer.layer = 10
	var rect := ColorRect.new()
	rect.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	rect.color = Color(0.9, 0.1, 0.1, 1.0)
	layer.add_child(rect)
	add_child(layer)

	# Setup gap (excluded from final video): RED
	await rec.wait_frames(8)

	# Clip 1: GREEN (10 frames)
	rect.color = Color(0.1, 0.9, 0.1, 1.0)
	rec.set_caption("Clip 1: Green")
	rec.begin_clip("green", "Green Clip")
	await rec.wait_frames(5)
	rec.mark("mid_green")
	await rec.wait_frames(5)
	rec.end_clip()

	# Inter-clip gap (excluded from final video): BLUE
	rec.clear_caption()
	rect.color = Color(0.1, 0.1, 0.9, 1.0)
	await rec.wait_frames(6)

	# Clip 2: YELLOW (10 frames)
	rect.color = Color(0.95, 0.95, 0.1, 1.0)
	rec.set_caption("Clip 2: Yellow")
	rec.begin_clip("yellow", "Yellow Clip")
	await rec.wait_frames(5)
	rec.mark("mid_yellow")
	await rec.wait_frames(5)
	rec.end_clip()

	rec.finish(0)
