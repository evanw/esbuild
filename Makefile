ESBUILD_VERSION = $(shell cat version.txt)

esbuild: src/esbuild/*/*.go
	cd src/esbuild && go build -o ../../esbuild ./main

test:
	cd src/esbuild && go test ./...
	make -j3 verify-source-map end-to-end-tests js-api-tests

test-wasm:
	cd src/esbuild && PATH="$(shell go env GOROOT)/misc/wasm:$$PATH" GOOS=js GOARCH=wasm go test ./...

verify-source-map: | node_modules
	node scripts/verify-source-map.js

end-to-end-tests:
	node scripts/end-to-end-tests.js

js-api-tests:
	node scripts/js-api-tests.js

update-version-go:
	echo "package main\n\nconst esbuildVersion = \"$(ESBUILD_VERSION)\"" > src/esbuild/main/version.go

platform-all: update-version-go test test-wasm
	make -j5 platform-windows platform-darwin platform-linux platform-wasm platform-neutral

platform-windows:
	cd npm/esbuild-windows-64 && npm version "$(ESBUILD_VERSION)" --allow-same-version
	cd src/esbuild && GOOS=windows GOARCH=amd64 go build -o ../../npm/esbuild-windows-64/esbuild.exe ./main

platform-darwin:
	mkdir -p npm/esbuild-darwin-64/bin
	cd npm/esbuild-darwin-64 && npm version "$(ESBUILD_VERSION)" --allow-same-version
	cd src/esbuild && GOOS=darwin GOARCH=amd64 go build -o ../../npm/esbuild-darwin-64/bin/esbuild ./main

platform-linux:
	mkdir -p npm/esbuild-linux-64/bin
	cd npm/esbuild-linux-64 && npm version "$(ESBUILD_VERSION)" --allow-same-version
	cd src/esbuild && GOOS=linux GOARCH=amd64 go build -o ../../npm/esbuild-linux-64/bin/esbuild ./main

platform-wasm:
	cd src/esbuild && GOOS=js GOARCH=wasm go build -o ../../npm/esbuild-wasm/esbuild.wasm ./main
	cd npm/esbuild-wasm && npm version "$(ESBUILD_VERSION)" --allow-same-version
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" npm/esbuild-wasm/wasm_exec.js

platform-neutral:
	cd npm/esbuild && npm version "$(ESBUILD_VERSION)" --allow-same-version

publish-all: update-version-go test test-wasm
	make -j5 publish-windows publish-darwin publish-linux publish-wasm publish-neutral

publish-windows: platform-windows
	[ ! -z "$(OTP)" ] && cd npm/esbuild-windows-64 && npm publish --otp="$(OTP)"

publish-darwin: platform-darwin
	[ ! -z "$(OTP)" ] && cd npm/esbuild-darwin-64 && npm publish --otp="$(OTP)"

publish-linux: platform-linux
	[ ! -z "$(OTP)" ] && cd npm/esbuild-linux-64 && npm publish --otp="$(OTP)"

publish-wasm: platform-wasm
	[ ! -z "$(OTP)" ] && cd npm/esbuild-wasm && npm publish --otp="$(OTP)"

publish-neutral: platform-neutral
	[ ! -z "$(OTP)" ] && cd npm/esbuild && npm publish --otp="$(OTP)"

clean:
	rm -f esbuild
	rm -f npm/esbuild-windows-64/esbuild.exe
	rm -rf npm/esbuild-darwin-64/bin
	rm -rf npm/esbuild-linux-64/bin
	rm -f npm/esbuild-wasm/esbuild.wasm npm/esbuild-wasm/wasm_exec.js
	cd src/esbuild && go clean -testcache ./...

node_modules:
	npm ci

################################################################################

github/test262:
	mkdir -p github
	git clone --depth 1 https://github.com/tc39/test262.git github/test262

demo/test262: | github/test262
	mkdir -p demo/test262
	cp -r github/test262/test demo/test262/test

test262: esbuild | demo/test262
	node scripts/test262.js

################################################################################

github/terser:
	mkdir -p github/terser
	cd github/terser && git init && git remote add origin https://github.com/terser/terser.git
	cd github/terser && git fetch --depth 1 origin 056623c20dbbc42d2f5a34926c07133981519326 && git checkout FETCH_HEAD

demo/terser: | github/terser
	mkdir -p demo/terser
	cp -r github/terser/ demo/terser
	cd demo/terser && npm ci && npm run build

terser: esbuild | demo/terser
	node scripts/terser-tests.js

################################################################################

github/three:
	mkdir -p github
	git clone --depth 1 --branch r108 https://github.com/mrdoob/three.js.git github/three

demo/three: | github/three
	mkdir -p demo/three
	cp -r github/three/src demo/three/src

bench/three: | github/three
	mkdir -p bench/three
	echo > bench/three/entry.js
	for i in 1 2 3 4 5 6 7 8 9 10; do test -d "bench/three/copy$$i" || cp -r github/three/src "bench/three/copy$$i"; done
	for i in 1 2 3 4 5 6 7 8 9 10; do echo "import * as copy$$i from './copy$$i/Three.js'; export {copy$$i}" >> bench/three/entry.js; done
	echo 'Line count:' && find bench/three -name '*.js' | xargs wc -l | tail -n 1

################################################################################

THREE_ROLLUP_CONFIG += import { terser } from 'rollup-plugin-terser';
THREE_ROLLUP_CONFIG += export default {
THREE_ROLLUP_CONFIG +=   output: { format: 'iife', name: 'THREE', sourcemap: true },
THREE_ROLLUP_CONFIG +=   plugins: [terser()],
THREE_ROLLUP_CONFIG += }

THREE_WEBPACK_FLAGS += --devtool=sourcemap
THREE_WEBPACK_FLAGS += --mode=production
THREE_WEBPACK_FLAGS += --output-library THREE

THREE_PARCEL_FLAGS += --global THREE
THREE_PARCEL_FLAGS += --no-autoinstall
THREE_PARCEL_FLAGS += --out-dir .
THREE_PARCEL_FLAGS += --public-url ./

THREE_FUSEBOX_RUN += require('fuse-box').fusebox({
THREE_FUSEBOX_RUN +=   target: 'browser',
THREE_FUSEBOX_RUN +=   entry: './fusebox-entry.js',
THREE_FUSEBOX_RUN +=   useSingleBundle: true,
THREE_FUSEBOX_RUN +=   output: './dist',
THREE_FUSEBOX_RUN += }).runProd();

demo-three: demo-three-esbuild demo-three-rollup demo-three-webpack demo-three-parcel demo-three-fusebox

demo-three-esbuild: esbuild | demo/three
	rm -fr demo/three/esbuild
	mkdir -p demo/three/esbuild
	cd demo/three/esbuild && time -p ../../../esbuild --bundle --name=THREE --sourcemap --minify ../src/Three.js --outfile=Three.esbuild.js
	du -h demo/three/esbuild/Three.esbuild.js*

demo-three-rollup: | node_modules demo/three
	rm -fr demo/three/rollup
	mkdir -p demo/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > demo/three/rollup/config.js
	cd demo/three/rollup && time -p ../../../node_modules/.bin/rollup ../src/Three.js -o Three.rollup.js -c config.js
	du -h demo/three/rollup/Three.rollup.js*

demo-three-webpack: | node_modules demo/three
	rm -fr demo/three/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p demo/three/webpack
	cd demo/three/webpack && time -p ../../../node_modules/.bin/webpack ../src/Three.js $(THREE_WEBPACK_FLAGS) -o Three.webpack.js
	du -h demo/three/webpack/Three.webpack.js*

demo-three-parcel: | node_modules demo/three
	rm -fr demo/three/parcel
	mkdir -p demo/three/parcel
	cd demo/three/parcel && time -p ../../../node_modules/.bin/parcel build ../src/Three.js $(THREE_PARCEL_FLAGS) --out-file Three.parcel.js
	du -h demo/three/parcel/Three.parcel.js*

demo-three-fusebox: | node_modules demo/three
	rm -fr demo/three/fusebox
	mkdir -p demo/three/fusebox
	echo "$(THREE_FUSEBOX_RUN)" > demo/three/fusebox/run.js
	echo 'import * as THREE from "../src/Three.js"; window.THREE = THREE' > demo/three/fusebox/fusebox-entry.js
	cd demo/three/fusebox && time -p node run.js
	du -h demo/three/fusebox/dist/app.js*

################################################################################

bench-three: bench-three-esbuild bench-three-rollup bench-three-webpack bench-three-parcel bench-three-fusebox

bench-three-esbuild: esbuild | bench/three
	rm -fr bench/three/esbuild
	mkdir -p bench/three/esbuild
	cd bench/three/esbuild && time -p ../../../esbuild --bundle --name=THREE --sourcemap --minify ../entry.js --outfile=entry.esbuild.js
	du -h bench/three/esbuild/entry.esbuild.js*

bench-three-rollup: | node_modules bench/three
	rm -fr bench/three/rollup
	mkdir -p bench/three/rollup
	echo "$(THREE_ROLLUP_CONFIG)" > bench/three/rollup/config.js
	cd bench/three/rollup && time -p ../../../node_modules/.bin/rollup ../entry.js -o entry.rollup.js -c config.js
	du -h bench/three/rollup/entry.rollup.js*

bench-three-webpack: | node_modules bench/three
	rm -fr bench/three/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p bench/three/webpack
	cd bench/three/webpack && time -p ../../../node_modules/.bin/webpack ../entry.js $(THREE_WEBPACK_FLAGS) -o entry.webpack.js
	du -h bench/three/webpack/entry.webpack.js*

bench-three-parcel: | node_modules bench/three
	rm -fr bench/three/parcel
	mkdir -p bench/three/parcel
	cd bench/three/parcel && time -p ../../../node_modules/.bin/parcel build ../entry.js $(THREE_PARCEL_FLAGS) --out-file entry.parcel.js
	du -h bench/three/parcel/entry.parcel.js*

bench-three-fusebox: | node_modules bench/three
	rm -fr bench/three/fusebox
	mkdir -p bench/three/fusebox
	echo "$(THREE_FUSEBOX_RUN)" > bench/three/fusebox/run.js
	echo 'import * as THREE from "../entry.js"; window.THREE = THREE' > bench/three/fusebox/fusebox-entry.js
	cd bench/three/fusebox && time -p node --max-old-space-size=8192 run.js
	du -h bench/three/fusebox/dist/app.js*

################################################################################

ROME_TSCONFIG += {
ROME_TSCONFIG +=   \"compilerOptions\": {
ROME_TSCONFIG +=     \"sourceMap\": true,
ROME_TSCONFIG +=     \"esModuleInterop\": true,
ROME_TSCONFIG +=     \"resolveJsonModule\": true,
ROME_TSCONFIG +=     \"moduleResolution\": \"node\",
ROME_TSCONFIG +=     \"target\": \"es2019\",
ROME_TSCONFIG +=     \"module\": \"commonjs\",
ROME_TSCONFIG +=     \"baseUrl\": \".\"
ROME_TSCONFIG +=   }
ROME_TSCONFIG += }

ROME_WEBPACK_CONFIG += module.exports = {
ROME_WEBPACK_CONFIG +=   entry: '../src/entry.ts',
ROME_WEBPACK_CONFIG +=   mode: 'production',
ROME_WEBPACK_CONFIG +=   target: 'node',
ROME_WEBPACK_CONFIG +=   devtool: 'sourcemap',
ROME_WEBPACK_CONFIG +=   module: { rules: [{ test: /\.ts$$/, loader: 'ts-loader', options: { transpileOnly: true } }] },
ROME_WEBPACK_CONFIG +=   resolve: {
ROME_WEBPACK_CONFIG +=     extensions: ['.ts', '.js'],
ROME_WEBPACK_CONFIG +=     alias: { rome: __dirname + '/../src/rome', '@romejs': __dirname + '/../src/@romejs' },
ROME_WEBPACK_CONFIG +=   },
ROME_WEBPACK_CONFIG +=   output: { filename: 'rome.webpack.js', path: __dirname },
ROME_WEBPACK_CONFIG += };

ROME_PARCEL_FLAGS += --bundle-node-modules
ROME_PARCEL_FLAGS += --no-autoinstall
ROME_PARCEL_FLAGS += --out-dir .
ROME_PARCEL_FLAGS += --public-url ./
ROME_PARCEL_FLAGS += --target node

github/rome:
	mkdir -p github/rome
	cd github/rome && git init && git remote add origin https://github.com/facebookexperimental/rome.git
	cd github/rome && git fetch --depth 1 origin d95a3a7aab90773c9b36d9c82a08c8c4c6b68aa5 && git checkout FETCH_HEAD

bench/rome: | github/rome
	mkdir -p bench/rome
	cp -r github/rome/packages bench/rome/src
	echo "$(ROME_TSCONFIG)" > bench/rome/src/tsconfig.json
	echo 'import "rome/bin/rome"' > bench/rome/src/entry.ts

	# Patch a cyclic import ordering issue that affects commonjs-style bundlers (webpack and parcel)
	echo "export { default as createHook } from './api/createHook';" > .temp
	sed "/createHook/d" bench/rome/src/@romejs/js-compiler/index.ts >> .temp
	mv .temp bench/rome/src/@romejs/js-compiler/index.ts

	# Fix a bug where parcel doesn't know about one specific node builtin module
	mkdir -p bench/rome/src/node_modules/inspector
	touch bench/rome/src/node_modules/inspector/index.js

	# These aliases are required to fix parcel path resolution
	echo '{ "alias": {' > bench/rome/src/package.json
	ls bench/rome/src/@romejs | sed 's/.*/"\@romejs\/&": ".\/@romejs\/&",/g' >> bench/rome/src/package.json
	echo '"rome": "./rome" }}' >> bench/rome/src/package.json

	# Get an approximate line count
	rm -r bench/rome/src/@romejs/js-parser/test-fixtures
	echo 'Line count:' && (find bench/rome/src -name '*.ts' && find bench/rome/src -name '*.js') | xargs wc -l | tail -n 1

################################################################################

bench-rome: bench-rome-esbuild bench-rome-webpack bench-rome-parcel

bench-rome-esbuild: esbuild | bench/rome
	rm -fr bench/rome/esbuild
	mkdir -p bench/rome/esbuild
	cd bench/rome/esbuild && time -p ../../../esbuild --bundle --sourcemap --minify ../src/entry.ts --outfile=rome.esbuild.js --platform=node
	du -h bench/rome/esbuild/rome.esbuild.js*

bench-rome-webpack: | node_modules bench/rome
	rm -fr bench/rome/webpack node_modules/.cache/terser-webpack-plugin
	mkdir -p bench/rome/webpack
	echo "$(ROME_WEBPACK_CONFIG)" > bench/rome/webpack/webpack.config.js
	cd bench/rome/webpack && time -p ../../../node_modules/.bin/webpack
	du -h bench/rome/webpack/rome.webpack.js*

bench-rome-parcel: | node_modules bench/rome
	rm -fr bench/rome/parcel
	mkdir -p bench/rome/parcel
	cd bench/rome/parcel && time -p ../../../node_modules/.bin/parcel build ../src/entry.ts $(ROME_PARCEL_FLAGS) --out-file rome.parcel.js
	du -h bench/rome/parcel/rome.parcel.js*
