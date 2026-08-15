# ConsoleParser — tokenisation + cursor-slot detection.
# Stateless; all methods static. Input is already lowercased by callers
# that need case-insensitive matching (the dispatcher does this).
#
# Cursor-state Dictionary keys: "slot", "cmd", "subcmd", "param_index", "prefix".
class_name ConsoleParser
extends RefCounted

# Tokenise with double-quoted-string support. Quoted substrings become
# single tokens with their surrounding quotes stripped. An unmatched
# opening quote swallows the rest into one token (shell-like). Backslash
# escapes are NOT supported.
static func _tokenise(input: String) -> Array[String]:
	var tokens: Array[String] = []
	var buf := ""
	var in_quotes := false
	var i := 0
	var s := input.strip_edges()
	while i < s.length():
		var c := s[i]
		if in_quotes:
			if c == "\"":
				in_quotes = false
			else:
				buf += c
		elif c == "\"":
			in_quotes = true
		elif c == " ":
			if buf.length() > 0:
				tokens.append(buf)
				buf = ""
		else:
			buf += c
		i += 1
	if buf.length() > 0:
		tokens.append(buf)
	return tokens


# parse() produces the structure consumed by the dispatcher:
#   { cmd: String, subcmd_candidate: String, positionals: Array[String] }
# subcmd_candidate is the raw second token; the registry decides whether
# it's a valid subcommand or should be treated as the first positional of
# a flat command.
static func parse(input: String) -> Dictionary:
	var parts := _tokenise(input)
	if parts.is_empty():
		return {"cmd": "", "subcmd_candidate": "", "positionals": []}
	var cmd: String = parts[0]
	var subcmd: String = parts[1] if parts.size() > 1 else ""
	var positionals: Array = []
	if parts.size() > 2:
		positionals = Array(parts.slice(2))
	return {"cmd": cmd, "subcmd_candidate": subcmd, "positionals": positionals}


# describe_cursor() returns a slot descriptor for the completion flow:
#   slot = "cmd":        { slot, prefix }
#   slot = "subcmd":     { slot, cmd, prefix }
#   slot = "positional": { slot, cmd, subcmd, param_index, prefix }
# positional.subcmd is "" when the command turned out to be flat — the
# completer resolves final flat-vs-category decision via the registry.
static func describe_cursor(input: String, cursor: int) -> Dictionary:
	var left := input.substr(0, clampi(cursor, 0, input.length()))
	var trailing_space := left.ends_with(" ")
	var tokens := _tokenise(left.strip_edges(false, true))
	var in_progress := ""
	if not trailing_space and tokens.size() > 0:
		in_progress = tokens.pop_back()

	var token_count := tokens.size()
	if token_count == 0:
		return {"slot": "cmd", "prefix": in_progress}

	if token_count == 1:
		return {"slot": "subcmd", "cmd": tokens[0], "prefix": in_progress}

	return {
		"slot": "positional",
		"cmd": tokens[0],
		"subcmd": tokens[1],
		"param_index": token_count - 2,
		"prefix": in_progress,
	}
