#!/bin/sh
# release.sh VERSION - build pb for every platform and zip it with the agent kit into dist/.
# Run by .github/workflows/release.yml on a version tag; works locally too (needs Go and zip).
set -eu
VERSION=${1:?usage: scripts/release.sh v0.1.0}
rm -rf dist && mkdir -p dist
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
	os=${target%/*}; arch=${target#*/}
	name=vibe-portfolio-builder-$VERSION-$os-$arch
	kit=dist/$name
	mkdir -p "$kit"
	exe=pb; [ "$os" = windows ] && exe=pb.exe
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$kit/$exe" .
	# The agent kit: instructions, skills (links resolved, so every assistant finds them), agents.
	cp README.md LICENSE NOTICE AGENTS.md CLAUDE.md .env.example "$kit/"
	mkdir -p "$kit/.claude" "$kit/.grok"
	cp -RL .agents "$kit/.agents"
	cp -RL .agents/skills "$kit/.claude/skills"
	cp -RL .agents/skills "$kit/.grok/skills"
	cp -R .claude/agents "$kit/.claude/agents"
	cp .claude/settings.json "$kit/.claude/settings.json"
	mkdir -p "$kit/data"
	(cd dist && zip -qr "$name.zip" "$name")
	rm -rf "$kit"
done
(cd dist && shasum -a 256 *.zip > checksums.txt)
ls -la dist
