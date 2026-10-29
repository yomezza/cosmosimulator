export class ApiError extends Error {
  constructor(message, { status = 0, kind = "http", cause } = {}) {
    super(message, { cause }); this.name = "ApiError"; this.status = status; this.kind = kind;
  }
}

export async function request(path, body, signal) {
  let response;
  try {
    response = await fetch("/api/" + path, {
      signal,
      method: body === undefined ? "GET" : "POST",
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (error) {
    if (error.name === "AbortError") throw error;
    throw new ApiError("Не удалось подключиться", { kind: "network", cause: error });
  }
  let data;
  try { data = JSON.parse(await response.text()); }
  catch (error) {
    throw new ApiError("Ошибка сервера", {
      status: response.status, kind: response.ok ? "protocol" : "http", cause: error,
    });
  }
  if (!response.ok) throw new ApiError(data?.error || "Ошибка сервера", { status: response.status });
  return data;
}
