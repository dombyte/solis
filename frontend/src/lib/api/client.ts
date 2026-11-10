// Solis API Client (sources in data.ts are full paths, e.g. /api/data/solis_status)
/** The server's message from its error body ({ error, message, code }), else the status. */
async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { message?: unknown };
    if (typeof body.message === 'string' && body.message) return body.message;
  } catch {
    // not JSON: fall through to the status
  }
  return `Request failed (${response.status})`;
}

class SolisApiClient {
  // Generic method to fetch data from any source endpoint
  async get(source: string, params?: Record<string, string>, options?: { signal?: AbortSignal }): Promise<unknown> {
    let url = source;
    if (params) {
      const searchParams = new URLSearchParams(params);
      url += `?${searchParams.toString()}`;
    }
    const response = await fetch(url, { signal: options?.signal });
    if (!response.ok) {
      throw new Error(await errorMessage(response));
    }
    return response.json();
  }
}

// Singleton instance
export const api = new SolisApiClient();
