class_name DebugBridgeProtocol
extends RefCounted

## Game-agnostic debug bridge IPC protocol. Defines file names and parses
## structured JSON requests while preserving legacy plain-text commands.

const COMMAND_FILE_NAME := "command"
const RESULT_FILE_NAME := "result"
const TEMP_SCRIPT_FILE_NAME := "temp_script.gd"
const VERSION := 1

const KIND_JSON := "json"
const KIND_JSON_INVALID := "json_invalid"
const KIND_PLAIN := "plain"


## Returns a Dictionary with kind/data/raw. Object-shaped JSON uses the
## structured path, malformed object-shaped input is json_invalid, and all
## remaining content is a legacy plain-text command.
static func parse_command_content(content: String) -> Dictionary:
	var stripped := content.strip_edges()
	if stripped.begins_with("{"):
		var parser := JSON.new()
		if parser.parse(stripped) == OK and parser.data is Dictionary:
			return {"kind": KIND_JSON, "data": parser.data, "raw": stripped}
		return {"kind": KIND_JSON_INVALID, "data": null, "raw": stripped}
	return {"kind": KIND_PLAIN, "data": stripped, "raw": stripped}


## Serialize a versioned result envelope for the IPC result file.
static func stringify_result(data: Dictionary) -> String:
	var envelope := data.duplicate()
	envelope["protocol"] = VERSION
	return JSON.stringify(envelope)
