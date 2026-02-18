function isEsbuildError(err) {
    return typeof err === "object" && err !== null && ("errors" in err || "message" in err);
}
export function formatErrorResponse(err) {
    if (isEsbuildError(err)) {
        return {
            content: [{
                    type: "text",
                    text: JSON.stringify({
                        errors: err.errors ?? [{ text: err.message ?? String(err) }],
                        warnings: err.warnings ?? [],
                    }, null, 2),
                }],
            isError: true,
        };
    }
    return {
        content: [{
                type: "text",
                text: JSON.stringify({
                    errors: [{ text: String(err) }],
                    warnings: [],
                }, null, 2),
            }],
        isError: true,
    };
}
