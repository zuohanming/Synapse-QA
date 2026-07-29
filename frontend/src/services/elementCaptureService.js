import { request, toQuery } from "./httpClient.js";

const capturePath = "/ui/page-elements/capture-sessions";
const elementPath = "/ui/page-elements";
const sessionFields = [
  "id",
  "pageId",
  "executorId",
  "browserChannel",
  "status",
  "mode",
  "candidateCount",
  "lastHeartbeatAt",
  "interruptedAt",
  "recoveryExpiresAt",
  "expiresAt"
];

function resourceID(value) {
  return encodeURIComponent(String(value));
}

function optionsWithSignal(method, body, signal) {
  const options = { method, signal };
  if (body !== undefined) {
    options.body = JSON.stringify(body);
  }
  return options;
}

async function withCandidateIssues(operation) {
  try {
    return await operation();
  } catch (error) {
    if (error?.status === 409 && Array.isArray(error.data?.issues)) {
      error.issues = error.data.issues;
    }
    throw error;
  }
}

function publicSession(data) {
  const source = data?.session || data || {};
  return Object.fromEntries(
    sessionFields
      .filter((field) => source[field] !== undefined)
      .map((field) => [field, source[field]])
  );
}

export const elementCaptureService = {
  async create(body, { signal } = {}) {
    const data = await request(capturePath, optionsWithSignal("POST", body, signal));
    return publicSession(data);
  },

  get(id, { signal } = {}) {
    return request(`${capturePath}/${resourceID(id)}`, { signal });
  },

  mode(id, mode, { signal } = {}) {
    return request(
      `${capturePath}/${resourceID(id)}/mode`,
      optionsWithSignal("PATCH", { mode }, signal)
    );
  },

  stop(id, { signal } = {}) {
    return request(
      `${capturePath}/${resourceID(id)}/stop`,
      optionsWithSignal("POST", undefined, signal)
    );
  },

  candidates(id, { afterId = 0, limit = 100, signal } = {}) {
    const safeAfterID = Math.max(0, Number(afterId) || 0);
    const safeLimit = Math.min(200, Math.max(1, Number(limit) || 100));
    return request(
      `${capturePath}/${resourceID(id)}/candidates${toQuery({
        afterId: safeAfterID,
        limit: safeLimit
      })}`,
      { signal }
    );
  },

  update(id, candidateId, body, { signal } = {}) {
    return withCandidateIssues(() => request(
      `${capturePath}/${resourceID(id)}/candidates/${resourceID(candidateId)}`,
      optionsWithSignal("PATCH", body, signal)
    ));
  },

  save(id, items, { signal } = {}) {
    return withCandidateIssues(() => request(
      `${capturePath}/${resourceID(id)}/save`,
      optionsWithSignal("POST", { items }, signal)
    ));
  },

  versions(elementId, { signal } = {}) {
    return request(`${elementPath}/${resourceID(elementId)}/versions`, { signal });
  },

  rollback(elementId, version, { signal } = {}) {
    return request(
      `${elementPath}/${resourceID(elementId)}/versions/${resourceID(version)}/rollback`,
      optionsWithSignal("POST", undefined, signal)
    );
  }
};
