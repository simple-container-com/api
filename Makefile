.PHONY: build clean

build:
	@echo "🔨 Building sc..."
	@go build -ldflags="-s -w" -o bin/sc ./cmd/sc
	@echo "🔐 Signing binary (macOS)..."
	@codesign -f -s - bin/sc 2>/dev/null || true
	@echo "🧹 Clearing Gatekeeper attributes..."
	@xattr -c bin/sc 2>/dev/null || true
	@xattr -cr bin/ 2>/dev/null || true
	@echo "✅ Build complete: bin/sc"
	@echo ""
	@echo "⚠️  NOTE: First run after rebuild may be slow (10-15s) due to macOS Gatekeeper."
	@echo "   This is normal - second run will be instant!"

clean:
	rm -f bin/sc

rebuild: clean build
