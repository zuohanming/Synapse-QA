import { request, toQuery } from "./httpClient.js";

export const dashboardService = {
  overview: (params = {}) => request(`/dashboard/overview${toQuery(params)}`)
};
