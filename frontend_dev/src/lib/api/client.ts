const API_BASE = '/api';

// Solis API Client
class SolisApiClient {
  private readonly baseUrl: string;

  constructor(baseUrl: string = API_BASE) {
    this.baseUrl = baseUrl;
  }

  // Health check
  async health(): Promise<Record<string, unknown>> {
    const response = await fetch('/health');
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json() as Promise<Record<string, unknown>>;
  }

  // Get all register keys
  async getKeys(): Promise<string[]> {
    const response = await fetch(`${this.baseUrl}/keys`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json() as Promise<string[]>;
  }

  // Generic method to fetch data from any source endpoint
  async get(source: string, params?: Record<string, string>): Promise<unknown> {
    let url = source;
    if (params) {
      const searchParams = new URLSearchParams(params);
      url += `?${searchParams.toString()}`;
    }
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json();
  }
}

// Singleton instance
export const api = new SolisApiClient();
