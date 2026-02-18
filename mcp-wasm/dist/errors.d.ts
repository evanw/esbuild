export declare function formatErrorResponse(err: unknown): {
    content: {
        type: "text";
        text: string;
    }[];
    isError: true;
};
