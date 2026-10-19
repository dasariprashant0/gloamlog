#!/usr/bin/env python3
"""Gloamlog helper for Claude Code: lets Claude write a short work-log entry into your Gloamlog journal.

It is a tiny MCP server (standard library only, no network). Its one tool, `log_work`, drops a small JSON file in
~/Library/Application Support/Gloamlog/inbox; the Gloamlog app picks it up and adds it to today's page, in the Work or
Personal list of the folder the entry names (Settings > Page).
Set up with:  claude mcp add --scope user gloamlog -- python3 <path to this file>

Claude Code starts this helper once per session and keeps it running, so an app update would otherwise reach a session only
after it restarts. The helper therefore reloads itself (re-runs this file) when the file on disk has changed, between requests.
"""
import json, os, sys, tempfile, time, uuid

INBOX = os.environ.get("GLOAMLOG_INBOX") or os.path.expanduser("~/Library/Application Support/Gloamlog/inbox")
TOOL = {
    "name": "log_work",
    "description": (
        "Write one entry in the user's Gloamlog work journal. Call it once when a meaningful piece of work is done "
        "(a problem solved, a deliverable finished, a decision made, or when the user says they are done), not after every step. "
        "Write 1-3 short plain sentences in the user's voice: the problem being solved, what was done, and what is still open. "
        "No secrets, tokens, customer data, file dumps or code. "
        "Pass `folder`: the absolute path of the project you are working in (your working directory); Gloamlog files the entry "
        "under Work or Personal by it."),
    "inputSchema": {"type": "object", "properties": {
        "summary": {"type": "string", "description": "The entry, 1-3 short sentences."},
        "folder": {"type": "string", "description": "Absolute path of the project folder you are working in."},
    }, "required": ["summary"]},
}
SELF = os.path.abspath(__file__)


def stamp():
    """The file's identity on disk (None when it can't be read): a change means the app was updated."""
    try:
        st = os.stat(SELF)
        return (st.st_mtime_ns, st.st_size)
    except OSError:
        return None


STARTED = stamp()


def folder_of(given):
    """The entry's folder: the one Claude names when it is a real absolute folder, else where this helper runs ("" if gone)."""
    if isinstance(given, str) and os.path.isabs(given) and os.path.isdir(given):
        return os.path.normpath(given)
    try:
        return os.getcwd()
    except OSError:
        return ""                                                             # gone: the app uses the list for notes without one


def write_entry(text, folder=None):
    text = " ".join(str(text).split())[:1500]
    if not text:
        return False
    cwd = folder_of(folder)                                                   # read before writing: a deleted folder strands nothing
    os.makedirs(INBOX, exist_ok=True)
    name = "%d-%s.json" % (int(time.time() * 1000), uuid.uuid4().hex[:8])
    fd, tmp = tempfile.mkstemp(prefix=".", suffix=".tmp", dir=INBOX)          # hidden until complete, then renamed
    with os.fdopen(fd, "w") as f:
        json.dump({"text": text, "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "cwd": cwd}, f)
    os.replace(tmp, os.path.join(INBOX, name))
    return True


def reply(i, result=None, error=None):
    msg = {"jsonrpc": "2.0", "id": i}
    msg["error" if error else "result"] = error or result
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def handle(m):
    i, method = m.get("id"), m.get("method")
    if i is None:                                                             # notifications need no answer
        return
    if method == "initialize":
        reply(i, {"protocolVersion": (m.get("params") or {}).get("protocolVersion", "2024-11-05"),
                  "capabilities": {"tools": {}}, "serverInfo": {"name": "gloamlog", "version": "1.1"}})
    elif method == "tools/list":
        reply(i, {"tools": [TOOL]})
    elif method == "tools/call":
        p = m.get("params") or {}
        if p.get("name") != "log_work":
            reply(i, error={"code": -32602, "message": "unknown tool"})
            return
        args = p.get("arguments") or {}
        try:
            ok = write_entry(args.get("summary", ""), args.get("folder"))
            reply(i, {"content": [{"type": "text", "text": "Added to Gloamlog." if ok else "Nothing to add."}]})
        except OSError as e:
            reply(i, {"content": [{"type": "text", "text": "Could not write to Gloamlog: %s" % e}], "isError": True})
    elif method == "ping":
        reply(i, {})
    else:
        reply(i, error={"code": -32601, "message": "method not found"})


def lines(fd=0):
    """Lines from stdin, read without Python's read-ahead: nothing is held in a buffer when the helper reloads itself."""
    buf = b""
    while True:
        while b"\n" not in buf:
            chunk = os.read(fd, 65536)
            if not chunk:
                if buf:
                    yield buf.decode("utf-8", "replace"), b""
                return
            buf += chunk
        line, buf = buf.split(b"\n", 1)
        yield line.decode("utf-8", "replace"), buf


def main():
    for line, rest in lines():
        try:
            m = json.loads(line)
        except ValueError:
            m = None
        if isinstance(m, dict):
            handle(m)
        now = stamp()
        if not rest and now is not None and now != STARTED:                  # updated, and no request waiting: run the new file
            try:
                os.execv(sys.executable, [sys.executable, SELF])
            except OSError:
                pass                                                          # keep serving with this version


if __name__ == "__main__":
    main()
