# ConsoleContext — minimal game-agnostic console context owned by the addon.
# Generic handlers receive this context with only the console service
# back-reference. Host projects supply domain-specific contexts alongside their command modules via
# ConsoleRegistry's context injection; the addon core imports no host-project
# path or type.
extends RefCounted

# Back-reference to the console service that owns the registry.
var service: Object


func _init(service: Object) -> void:
	self.service = service
