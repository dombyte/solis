import type {
  RegisterInfo,
  HistoryResult,
  DailyDataPoint,
  MonthlyDataPoint,
  YearlyDataPoint,
  TotalDataPoint,
} from "../types";

const API_BASE = "/api";

export class SolisApiClient {
  private readonly baseUrl: string;

  constructor(baseUrl: string = API_BASE) {
    this.baseUrl = baseUrl;
  }

  // Health check
  async health(): Promise<any> {
    const response = await fetch("/health");
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json();
  }

  // Get all register keys
  async getKeys(): Promise<RegisterInfo[]> {
    const response = await fetch(`${this.baseUrl}/keys`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json() as Promise<RegisterInfo[]>;
  }



  // Get register data (current or historical)
  async getData(
    key: string,
    start?: string,
    end?: string,
  ): Promise<HistoryResult> {
    let url = `/data/${key}`;
    const params = new URLSearchParams();
    if (start) params.append("start", start);
    if (end) params.append("end", end);
    if (params.toString()) url += `?${params.toString()}`;

    const response = await fetch(`${this.baseUrl}${url}`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();

    // Handle different response formats
    // 1. If it's an array, wrap it as HistoryResult
    if (Array.isArray(data)) {
      return {
        data: data as any[],
        interval: "raw",
        key: key,
        unit: "",
      };
    }

    // 2. If it's an object with a 'data' property (HistoryResult or { data: [...] })
    if (typeof data === "object" && data !== null && "data" in data) {
      return data as HistoryResult;
    }

    // 3. If it's a single HistoryDataPoint object
    if (
      typeof data === "object" &&
      data !== null &&
      "value" in data &&
      "timestamp" in data
    ) {
      return {
        data: [data] as any[],
        interval: "raw",
        key: key,
        unit: "",
      };
    }

    // 4. Fallback: wrap whatever we got
    return {
      data: [data] as any[],
      interval: "raw",
      key: key,
      unit: "",
    };
  }

  // Get daily values for a register
  async getDaily(
    key: string,
    start?: string,
    end?: string,
  ): Promise<DailyDataPoint[]> {
    let url = `/history/daily/${key}`;
    const params = new URLSearchParams();
    if (start) params.append("start", start);
    if (end) params.append("end", end);
    if (params.toString()) url += `?${params.toString()}`;

    const response = await fetch(`${this.baseUrl}${url}`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();
    return data as DailyDataPoint[];
  }

  // Get monthly values for a register
  async getMonthly(
    key: string,
    start?: string,
    end?: string,
  ): Promise<MonthlyDataPoint[]> {
    let url = `/history/monthly/${key}`;
    const params = new URLSearchParams();
    if (start) params.append("start", start);
    if (end) params.append("end", end);
    if (params.toString()) url += `?${params.toString()}`;

    const response = await fetch(`${this.baseUrl}${url}`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();
    return data as MonthlyDataPoint[];
  }

  // Get yearly values for a register
  async getYearly(
    key: string,
    start?: string,
    end?: string,
  ): Promise<YearlyDataPoint[]> {
    let url = `/history/yearly/${key}`;
    const params = new URLSearchParams();
    if (start) params.append("start", start);
    if (end) params.append("end", end);
    if (params.toString()) url += `?${params.toString()}`;

    const response = await fetch(`${this.baseUrl}${url}`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    const data = await response.json();
    return data as YearlyDataPoint[];
  }

  // Get total (lifetime) value for a register
  async getTotal(key: string): Promise<TotalDataPoint> {
    const response = await fetch(`${this.baseUrl}/history/total/${key}`);
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }
    return response.json() as Promise<TotalDataPoint>;
  }


}

// Export a singleton instance
export const api = new SolisApiClient();
