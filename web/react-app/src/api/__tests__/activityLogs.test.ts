import { activityLogsAPI, ActivityLogsFilters } from '../activityLogs';
import apiClient from '../client';

// Mock the apiClient
jest.mock('../client');
const mockedApiClient = apiClient as jest.Mocked<typeof apiClient>;

describe('ActivityLogsAPI', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('getActivityLogs', () => {
    it('should construct request with no filters', async () => {
      const mockResponse = {
        status: 'success',
        data: {
          logs: [],
          pagination: {
            page: 1,
            page_size: 20,
            total: 0,
            total_pages: 0,
            has_next: false,
            has_prev: false,
          },
        },
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getActivityLogs();

      expect(mockedApiClient.get).toHaveBeenCalledWith('/activity-logs');
      expect(result).toEqual(mockResponse);
    });

    it('should construct request with all filters', async () => {
      const filters: ActivityLogsFilters = {
        level: 'INFO',
        action: 'start_process',
        resource: 'process',
        username: 'admin',
        start_time: '2024-01-01T00:00:00Z',
        end_time: '2024-01-31T23:59:59Z',
        page: 2,
        page_size: 50,
      };

      const mockResponse = {
        status: 'success',
        data: {
          logs: [],
          pagination: {
            page: 2,
            page_size: 50,
            total: 100,
            total_pages: 2,
            has_next: false,
            has_prev: true,
          },
        },
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getActivityLogs(filters);

      expect(mockedApiClient.get).toHaveBeenCalledWith(
        '/activity-logs?level=INFO&action=start_process&resource=process&username=admin&start_time=2024-01-01T00:00:00Z&end_time=2024-01-31T23:59:59Z&page=2&page_size=50'
      );
      expect(result).toEqual(mockResponse);
    });

    it('should construct request with partial filters', async () => {
      const filters: ActivityLogsFilters = {
        level: 'ERROR',
        username: 'john',
      };

      const mockResponse = {
        status: 'success',
        data: {
          logs: [],
          pagination: {
            page: 1,
            page_size: 20,
            total: 5,
            total_pages: 1,
            has_next: false,
            has_prev: false,
          },
        },
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getActivityLogs(filters);

      expect(mockedApiClient.get).toHaveBeenCalledWith(
        '/activity-logs?level=ERROR&username=john'
      );
      expect(result).toEqual(mockResponse);
    });

    it('should handle API errors correctly', async () => {
      const error = new Error('Network error');
      mockedApiClient.get.mockRejectedValue(error);

      await expect(activityLogsAPI.getActivityLogs()).rejects.toThrow('Network error');
    });
  });

  describe('getRecentLogs', () => {
    it('should request recent logs with default limit', async () => {
      const mockResponse = {
        status: 'success',
        logs: [
          {
            id: 1,
            created_at: '2024-01-01T12:00:00Z',
            level: 'INFO',
            message: 'Test log',
            action: 'test',
            resource: 'test',
            target: 'test',
            user_id: '1',
            username: 'admin',
            ip_address: '127.0.0.1',
            user_agent: 'test',
            details: '',
            status: 'success',
            duration: 0,
          },
        ],
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getRecentLogs();

      expect(mockedApiClient.get).toHaveBeenCalledWith('/activity-logs/recent?limit=20');
      expect(result).toEqual(mockResponse.logs);
    });

    it('should request recent logs with custom limit', async () => {
      const mockResponse = {
        status: 'success',
        logs: [],
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getRecentLogs(50);

      expect(mockedApiClient.get).toHaveBeenCalledWith('/activity-logs/recent?limit=50');
      expect(result).toEqual(mockResponse.logs);
    });

    it('should handle API errors correctly', async () => {
      const error = new Error('Failed to fetch');
      mockedApiClient.get.mockRejectedValue(error);

      await expect(activityLogsAPI.getRecentLogs()).rejects.toThrow('Failed to fetch');
    });
  });

  describe('getLogStatistics', () => {
    it('should request statistics with default days', async () => {
      const mockResponse = {
        status: 'success',
        data: {
          total_logs: 100,
          info_count: 80,
          warning_count: 15,
          error_count: 5,
          debug_count: 0,
          top_actions: [],
          top_users: [],
        },
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getLogStatistics();

      expect(mockedApiClient.get).toHaveBeenCalledWith('/activity-logs/statistics?days=7');
      expect(result).toEqual(mockResponse.data);
    });

    it('should request statistics with custom days', async () => {
      const mockResponse = {
        status: 'success',
        data: {
          total_logs: 500,
          info_count: 400,
          warning_count: 75,
          error_count: 25,
          debug_count: 0,
          top_actions: [],
          top_users: [],
        },
      };

      mockedApiClient.get.mockResolvedValue(mockResponse);

      const result = await activityLogsAPI.getLogStatistics(30);

      expect(mockedApiClient.get).toHaveBeenCalledWith('/activity-logs/statistics?days=30');
      expect(result).toEqual(mockResponse.data);
    });

    it('should handle API errors correctly', async () => {
      const error = new Error('Server error');
      mockedApiClient.get.mockRejectedValue(error);

      await expect(activityLogsAPI.getLogStatistics()).rejects.toThrow('Server error');
    });
  });

  describe('exportLogs', () => {
    beforeEach(() => {
      // Mock localStorage
      Storage.prototype.getItem = jest.fn(() => 'test-token');
      
      // Mock fetch
      global.fetch = jest.fn();
    });

    afterEach(() => {
      jest.restoreAllMocks();
    });

    it('should export logs with no filters', async () => {
      const mockBlob = new Blob(['test'], { type: 'text/csv' });
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        blob: () => Promise.resolve(mockBlob),
      });

      const result = await activityLogsAPI.exportLogs();

      expect(global.fetch).toHaveBeenCalledWith(
        '/api/activity-logs/export',
        expect.objectContaining({
          method: 'GET',
          headers: {
            'Authorization': 'Bearer test-token',
          },
        })
      );
      expect(result).toEqual(mockBlob);
    });

    it('should export logs with filters', async () => {
      const filters: ActivityLogsFilters = {
        level: 'ERROR',
        action: 'stop_process',
        start_time: '2024-01-01T00:00:00Z',
      };

      const mockBlob = new Blob(['test'], { type: 'text/csv' });
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        blob: () => Promise.resolve(mockBlob),
      });

      const result = await activityLogsAPI.exportLogs(filters);

      expect(global.fetch).toHaveBeenCalledWith(
        '/api/activity-logs/export?level=ERROR&action=stop_process&start_time=2024-01-01T00:00:00Z',
        expect.objectContaining({
          method: 'GET',
          headers: {
            'Authorization': 'Bearer test-token',
          },
        })
      );
      expect(result).toEqual(mockBlob);
    });

    it('should handle export errors correctly', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'Backend export failed' }),
      });

      await expect(activityLogsAPI.exportLogs()).rejects.toThrow('Backend export failed');
    });

    it('should handle network errors correctly', async () => {
      (global.fetch as jest.Mock).mockRejectedValue(new Error('Network error'));

      await expect(activityLogsAPI.exportLogs()).rejects.toThrow('Network error');
    });
  });
});
