import axios from 'axios';
import { useAuth } from '../store/useAuth';

export const api = axios.create({
  baseURL: 'http://localhost:8082/api/v1',
});

api.interceptors.request.use((config) => {
  const token = useAuth.getState().accessToken;
  if (token) {
    config.headers.Authorization = 'Bearer ' + token;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;
      try {
        const refreshToken = useAuth.getState().refreshToken;
        const res = await axios.post('http://localhost:8082/api/v1/refresh', {
          refresh_token: refreshToken,
        });
        const { access_token, refresh_token } = res.data;
        useAuth.getState().login(access_token, refresh_token);
        originalRequest.headers.Authorization = 'Bearer ' + access_token;
        return api(originalRequest);
      } catch (err) {
        useAuth.getState().logout();
        return Promise.reject(err);
      }
    }
    return Promise.reject(error);
  }
);
