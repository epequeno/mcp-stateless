# Decisions

Short log of choices that would otherwise be re-litigated. Newest last.

## 2026-10-13 · Scaffold, and one binary with two modes

Two modes in one binary (`--pinned`) rather than two programs, so the benchmark
compares like with like: same registry, same routing code, same JSON encoding,
one behavioural difference. Two implementations would confound the comparison
with unrelated differences.

## 2026-10-13 · Method routing falls back to the body

`protocol.MethodFrom` prefers the header and falls back to the parsed body. The
header is what makes the core stateless; the fallback keeps a session-pinned
client working, which is what makes the A/B a comparison of *session handling*
rather than of two protocols.

Consequence to watch: the fallback means the server still reads the body. If a
later goal is to route without buffering, this is the line to revisit.

## 2026-10-13 · `Risk` on the registry, unused today

Every tool carries a `Risk` class now, though nothing enforces it yet. The
follow-on governance project classifies exactly these entries and requires every
reachable tool to be classified; adding the field later would mean touching every
call site. Cheaper to carry an unused string now.

## 2026-10-13 · MIT

Matches `support-triage`. The point of the repo is that a reviewer can read and
reuse it.
