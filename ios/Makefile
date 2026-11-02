SCHEME ?= Matchblox
DESTINATION ?= generic/platform=iOS Simulator
DERIVED ?= build

.PHONY: install
install: ## Install XcodeGen with Homebrew
	@command -v brew >/dev/null 2>&1 || { echo "Homebrew is required. Install it from https://brew.sh"; exit 1; }
	@command -v xcodegen >/dev/null 2>&1 || brew install xcodegen

DEMO_OUT := build/demolib

# demolib builds the demo, the real console and service as a C library, for a
# device and for the simulator (Apple silicon), and the tar of the machine it
# shows. Both go into the app. The tar is one file: the fixture holds symlinks
# that code signing may refuse in a folder.
.PHONY: demolib
demolib: machine ## Build build/MBDemo.xcframework and the demo's machine
	rm -rf build/MBDemo.xcframework $(DEMO_OUT)
	$(call demo-slice,iphoneos,arm64-apple-ios17.0,device)
	$(call demo-slice,iphonesimulator,arm64-apple-ios17.0-simulator,sim)
	xcodebuild -create-xcframework \
		-library $(DEMO_OUT)/device/libmbdemo.a -headers $(DEMO_OUT)/device/include \
		-library $(DEMO_OUT)/sim/libmbdemo.a -headers $(DEMO_OUT)/sim/include \
		-output build/MBDemo.xcframework

# demo-slice: sdk, clang target, folder.
define demo-slice
CC="$$(xcrun -sdk $(1) -f clang) -isysroot $$(xcrun -sdk $(1) --show-sdk-path) -target $(2)" \
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
	go -C .. build -trimpath -ldflags='-s -w' -buildmode=c-archive \
	-o "$(CURDIR)/$(DEMO_OUT)/$(3)/libmbdemo.a" ./cmd/mbdemolib
mkdir -p $(DEMO_OUT)/$(3)/include
mv $(DEMO_OUT)/$(3)/libmbdemo.h $(DEMO_OUT)/$(3)/include/
endef

.PHONY: machine
machine: ## Pack testdata/demo as Matchblox/Demo/machine.tar, symlinks kept
	COPYFILE_DISABLE=1 tar -C ../testdata/demo -cf Matchblox/Demo/machine.tar .

.PHONY: gen
gen: demolib ## Generate Matchblox.xcodeproj from project.yml
	@command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen not found. Run: make install"; exit 1; }
	xcodegen generate

.PHONY: build
build: gen ## Compile the app for the simulator, without code signing
	xcodebuild \
		-scheme $(SCHEME) \
		-destination '$(DESTINATION)' \
		-derivedDataPath $(DERIVED) \
		-skipPackagePluginValidation \
		CODE_SIGNING_ALLOWED=NO \
		build

.PHONY: release
release: gen ## Archive the app in Release as TestFlight does, without code signing
	xcodebuild \
		-scheme $(SCHEME) \
		-configuration Release \
		-destination 'generic/platform=iOS' \
		-derivedDataPath $(DERIVED) \
		-archivePath $(DERIVED)/Matchblox.xcarchive \
		-skipPackagePluginValidation \
		CODE_SIGNING_ALLOWED=NO \
		LD_GENERATE_MAP_FILE=YES \
		LD_MAP_FILE_PATH='$(CURDIR)/$(DERIVED)/$$(TARGET_NAME).linkmap' \
		archive

# The simulator's ad hoc signature is enough for the test runner.
.PHONY: uitest
uitest: gen ## Run the UI tests on the simulator named by DESTINATION, for example id=<udid>
	rm -rf $(DERIVED)/ui.xcresult
	xcodebuild \
		-scheme $(SCHEME) \
		-destination '$(DESTINATION)' \
		-derivedDataPath $(DERIVED) \
		-resultBundlePath $(DERIVED)/ui.xcresult \
		-skipPackagePluginValidation \
		CODE_SIGN_STYLE=Manual \
		CODE_SIGN_IDENTITY=- \
		test

.PHONY: icon
icon: ## Render the app icon from AppIcon.svg (needs rsvg-convert: brew install librsvg)
	@command -v rsvg-convert >/dev/null 2>&1 || { echo "rsvg-convert not found. Run: brew install librsvg"; exit 1; }
	mkdir -p $(DERIVED)
	rsvg-convert -w 1024 -h 1024 AppIcon.svg -o $(DERIVED)/AppIcon.png
	sips -s format jpeg -s formatOptions 100 $(DERIVED)/AppIcon.png --out $(DERIVED)/AppIcon.jpg >/dev/null
	sips -s format png $(DERIVED)/AppIcon.jpg --out Matchblox/Assets.xcassets/AppIcon.appiconset/AppIcon.png >/dev/null

.PHONY: test
test: ## Run the MatchbloxKit tests
	swift test --package-path MatchbloxKit
