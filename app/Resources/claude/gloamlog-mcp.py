#!/usr/bin/env python3
"""Gloamlog helper for Claude Code: lets Claude write a short work-log entry into your Gloamlog journal.

It is a tiny MCP server (standard library only, no network). Its one tool, `log_work`, drops a small JSON file in
~/Library/Application Support/Gloamlog/inbox; the Gloamlog app picks it up and adds it to today's page.
Set up with:  claude mcp add --scope user gloamlog -- python3 <path to this file>
"""
import json, os, sys, tempfile, time, uuid

INBOX = os.environ.get("GLOAMLOG_INBOX") or os.path.expanduser("~/Library/Application Support/Gloamlog/inbox")
TOOL = {
    "name": "log_work",
    "description": (
        "Write one entry in the user's Gloamlog work journal. Call it once when a meaningful piece of work is done "
        "(a problem solved, a deliverable finished, a decision made, or when the user says they are done), not after every step. "
        "Write 1-3 short plain sentences in the user's voice: the problem being solved, what was done, and what is still open. "
        "No secrets, tokens, customer data, file dumps or code."),
    "inputSchema": {"type": "object", "properties": {"summary": {"type": "string", "description": "The entry, 1-3 short sentences."}}, "required": ["summary"]},
}


def write_entry(text):
    text = " ".join(str(text).split())[:1500]
    if not text:
        return False
    os.makedirs(INBOX, exist_ok=True)
    name = "%d-%s.json" % (int(time.time() * 1000), uuid.uuid4().hex[:8])
    fd, tmp = tempfile.mkstemp(prefix=".", suffix=".tmp", dir=INBOX)          # hidden until complete, then renamed
    with os.fdopen(fd, "w") as f:
        json.dump({"text": text, "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}, f)
    os.replace(tmp, os.path.join(INBOX, name))
    return True


def reply(i, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": i}
    msg["error" if error else "result"] = error or result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def main():
    for line in sys.stdin:
        try:
            m = json.loads(line)
        except ValueError:
            continue
        i, method = m.get("id"), m.get("method")
        if i is None:                                                         # notifications need no answer
            continue
        if method == "initialize":
            reply(i, {"protocolVersion": (m.get("params") or {}).get("protocolVersion", "2024-11-05"),
                      "capabilities": {"tools": {}}, "serverInfo": {"name": "gloamlog", "version": "1.0"}})
        elif method == "tools/list":
            reply(i, {"tools": [TOOL]})
        elif method == "tools/call":
            p = m.get("params") or {}
            if p.get("name") != "log_work":
                reply(i, error={"code": -32602, "message": "unknown tool"})
                continue
            try:
                ok = write_entry((p.get("arguments") or {}).get("summary", ""))
                reply(i, {"content": [{"type": "text", "text": "Added to Gloamlog." if ok else "Nothing to add."}]})
            except OSError as e:
                reply(i, {"content": [{"type": "text", "text": "Could not write to Gloamlog: %s" % e}], "isError": True})
        elif method == "ping":
            reply(i, {})
        else:
            reply(i, error={"code": -32601, "message": "method not found"})


if __name__ == "__main__":
    main()
