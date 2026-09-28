// Solis API Client (sources in data.ts are full paths, e.g. /api/data/solis_status)
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
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json();
  }
}

// Singleton instance
export const api = new SolisApiClient();
