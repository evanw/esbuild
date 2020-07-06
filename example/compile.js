const svelte = require("svelte/compiler");

process.stdin.on("data", (buf) => {
	const result = svelte.compile(buf.toString("utf8"), {
		generate: "dom",
		hydratable: true,
		format: "esm",
	});
	console.log(result.js.code);
});
