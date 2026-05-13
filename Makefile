APP_NAME = Kiromon
APP_DIR = $(APP_NAME).app
CONTENTS_DIR = $(APP_DIR)/Contents
MACOS_DIR = $(CONTENTS_DIR)/MacOS
RESOURCES_DIR = $(CONTENTS_DIR)/Resources
BINARY_NAME = kiromon
ICON_NAME = icon

.PHONY: all clean build app install icon uninstall

all: clean build app

build:
	@echo "🛠️  Building Go binary..."
	go build -o $(BINARY_NAME) .

# 🌟 새로운 아이콘 생성 로직
icon:
	@if [ -f icon.png ]; then \
		echo "🎨  Generating App Icon..."; \
		mkdir -p $(ICON_NAME).iconset; \
		sips -z 16 16     icon.png --out $(ICON_NAME).iconset/icon_16x16.png > /dev/null; \
		sips -z 32 32     icon.png --out $(ICON_NAME).iconset/icon_16x16@2x.png > /dev/null; \
		sips -z 32 32     icon.png --out $(ICON_NAME).iconset/icon_32x32.png > /dev/null; \
		sips -z 64 64     icon.png --out $(ICON_NAME).iconset/icon_32x32@2x.png > /dev/null; \
		sips -z 128 128   icon.png --out $(ICON_NAME).iconset/icon_128x128.png > /dev/null; \
		sips -z 256 256   icon.png --out $(ICON_NAME).iconset/icon_128x128@2x.png > /dev/null; \
		sips -z 256 256   icon.png --out $(ICON_NAME).iconset/icon_256x256.png > /dev/null; \
		sips -z 512 512   icon.png --out $(ICON_NAME).iconset/icon_256x256@2x.png > /dev/null; \
		sips -z 512 512   icon.png --out $(ICON_NAME).iconset/icon_512x512.png > /dev/null; \
		sips -z 1024 1024 icon.png --out $(ICON_NAME).iconset/icon_512x512@2x.png > /dev/null; \
		iconutil -c icns $(ICON_NAME).iconset; \
		rm -rf $(ICON_NAME).iconset; \
	else \
		echo "⚠️  icon.png not found. Skipping icon generation."; \
	fi

app: build icon
	@echo "📦  Packaging $(APP_DIR)..."
	@mkdir -p $(MACOS_DIR)
	@mkdir -p $(RESOURCES_DIR)
	@cp $(BINARY_NAME) $(MACOS_DIR)/$(APP_NAME)
	@if [ -f $(ICON_NAME).icns ]; then \
		cp $(ICON_NAME).icns $(RESOURCES_DIR)/; \
	fi
	@echo "📝  Generating Info.plist..."
	@echo '<?xml version="1.0" encoding="UTF-8"?>\n\
	<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n\
	<plist version="1.0">\n\
	<dict>\n\
		<key>CFBundleExecutable</key>\n\
		<string>$(APP_NAME)</string>\n\
		<key>CFBundleIdentifier</key>\n\
		<string>com.kiro.menubar</string>\n\
		<key>CFBundleName</key>\n\
		<string>$(APP_NAME)</string>\n\
		<key>CFBundleIconFile</key>\n\
		<string>$(ICON_NAME)</string>\n\
		<key>LSUIElement</key>\n\
		<true/>\n\
	</dict>\n\
	</plist>' > $(CONTENTS_DIR)/Info.plist
	@echo "✅  Packaging complete!"

install: app
	@echo "🚀  Installing to ~/Applications..."
	@mkdir -p ~/Applications
	@rm -rf ~/Applications/$(APP_DIR)
	@cp -r $(APP_DIR) ~/Applications/
	@echo "🎉  Installed! You can now run $(APP_NAME) from Spotlight."

clean:
	@echo "🧹  Cleaning up..."
	@rm -f $(BINARY_NAME) $(ICON_NAME).icns
	@rm -rf $(APP_DIR) $(ICON_NAME).iconset

uninstall:
	@echo "🗑️  Uninstalling $(APP_NAME)..."
	@rm -rf ~/Applications/$(APP_DIR)
	@echo "✨  Uninstalled successfully!"