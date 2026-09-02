import apiClient from './client';

export type ExportType = 'users' | 'logs' | 'configs' | 'processes' | 'all';
export type ExportFormat = 'json' | 'csv';

export interface ExportRecord {
  id: string;
  name: string;
  export_type: ExportType;
  format: ExportFormat;
  file_path: string;
  file_size: number;
  record_count: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
  error_msg: string;
  created_by: string;
  created_at: string;
  completed_at?: string | null;
}

export interface ExportRecordsResponse {
  data: ExportRecord[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface CreateExportRequest {
  export_type: ExportType;
  format: ExportFormat;
}

export interface ImportRecord {
  id: string;
  name: string;
  import_type: 'configs';
  source_file: string;
  file_size: number;
  total_records: number;
  success_count: number;
  failure_count: number;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'partial';
  error_msg: string;
  validation_log: string;
  created_by: string;
  created_at: string;
  completed_at?: string | null;
}

export interface ImportRecordsResponse {
  data: ImportRecord[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export const exportFormatOptions: Record<ExportType, ExportFormat[]> = {
  users: ['json', 'csv'],
  logs: ['json', 'csv'],
  configs: ['json', 'csv'],
  processes: ['json'],
  all: ['json'],
};

export const dataManagementApi = {
  createExport: (data: CreateExportRequest) =>
    apiClient.post<ExportRecord>('/data-management/exports', data),

  getExportRecords: (page: number = 1, pageSize: number = 10) =>
    apiClient.get<ExportRecordsResponse>(`/data-management/exports?page=${page}&page_size=${pageSize}`),

  deleteExportRecord: (id: string) =>
    apiClient.delete<{ message: string }>(`/data-management/exports/${id}`),

  getImportRecords: (page: number = 1, pageSize: number = 10) =>
    apiClient.get<ImportRecordsResponse>(`/data-management/imports?page=${page}&page_size=${pageSize}`),

  deleteImportRecord: (id: string) =>
    apiClient.delete<{ message: string }>(`/data-management/imports/${id}`),

  async downloadExportFile(id: string): Promise<Blob> {
    const response = await fetch(`/api/data-management/exports/${id}/download`, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('token')}`,
      },
    });

    if (!response.ok) {
      throw new Error('Failed to download export');
    }

    return response.blob();
  },

  async importConfigurations(file: File, overwriteExisting: boolean): Promise<ImportRecord> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('import_type', 'configs');
    formData.append('overwrite_existing', String(overwriteExisting));

    const response = await fetch('/api/data-management/imports', {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${localStorage.getItem('token')}`,
      },
      body: formData,
    });

    if (!response.ok) {
      const payload = await response.json().catch(() => ({}));
      throw new Error(payload.error || 'Failed to import configurations');
    }

    return response.json();
  },
};

export default dataManagementApi;
