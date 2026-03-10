import axios, { AxiosInstance, AxiosResponse } from 'axios';
import { NextApiRequest, NextApiResponse } from 'next/api';

interface ApiClientOptions {
  apiUrl: string;
  apiToken: string;
}

class ApiClient {
  private axiosInstance: AxiosInstance;

  constructor(options: ApiClientOptions) {
    this.axiosInstance = axios.create({
      baseURL: options.apiUrl,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${options.apiToken}`,
      },
    });
  }

  public async get<T>(url: string): Promise<T> {
    return this.sendRequest('GET', url);
  }

  public async post<T>(url: string, data: any): Promise<T> {
    return this.sendRequest('POST', url, data);
  }

  public async put<T>(url: string, data: any): Promise<T> {
    return this.sendRequest('PUT', url, data);
  }

  public async delete<T>(url: string): Promise<T> {
    return this.sendRequest('DELETE', url);
  }

  private async sendRequest<T>(method: 'GET' | 'POST' | 'PUT' | 'DELETE', url: string, data?: any): Promise<T> {
    try {
      const response = await this.axiosInstance.request({
        method,
        url,
        data,
      });

      if (response.status >= 200 && response.status < 300) {
        return response.data;
      } else {
        throw new Error(`Error ${response.status}: ${response.statusText}`);
      }
    } catch (error) {
      throw error;
    }
  }
}

export default ApiClient;