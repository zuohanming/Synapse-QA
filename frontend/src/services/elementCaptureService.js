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

function resourceID(value, field = "id") {
  const normalized = String(value ?? "").trim();
  if (!normalized) throw new TypeError(`${field} 不能为空`);
  return encodeURIComponent(normalized);
}

function integerParam(value, field, { min = 1, max = Number.MAX_SAFE_INTEGER } = {}) {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < min || value > max) {
    throw new TypeError(`${field} 必须是 ${min} 到 ${max} 之间的整数`);
  }
  return value;
}

function validateSaveItems(items) {
  if (!Array.isArray(items) || items.length < 1 || items.length > 200) {
    throw new TypeError("items 数量必须在 1 到 200 之间");
  }
  return items.map((item, index) => {
    const candidateId = integerParam(item?.candidateId, `items[${index}].candidateId`);
    const resolution = String(item?.resolution || "");
    if (!["create", "update", "ignore"].includes(resolution)) {
      throw new TypeError(`items[${index}].resolution 无效`);
    }
    const normalized = { candidateId, resolution };
    if (resolution === "update") {
      normalized.targetElementId = integerParam(
        item?.targetElementId,
        `items[${index}].targetElementId`
      );
    }
    return normalized;
  });
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
    const payload = {
      ...body,
      pageId: integerParam(body?.pageId, "pageId")
    };
    const data = await request(capturePath, optionsWithSignal("POST", payload, signal));
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

  remove(id, candidateId, { signal } = {}) {
    return request(
      `${capturePath}/${resourceID(id)}/candidates/${resourceID(candidateId, "candidateId")}`,
      optionsWithSignal("DELETE", undefined, signal)
    );
  },

  candidates(id, { afterId = 0, limit = 100, signal } = {}) {
    const safeAfterID = integerParam(afterId, "afterId", { min: 0 });
    const safeLimit = integerParam(limit, "limit", { min: 1, max: 200 });
    return request(
      `${capturePath}/${resourceID(id)}/candidates${toQuery({
        afterId: safeAfterID,
        limit: safeLimit
      })}`,
      { signal }
    );
  },

  update(id, candidateId, body, { signal } = {}) {
    const safeCandidateID = integerParam(candidateId, "candidateId");
    return withCandidateIssues(() => request(
      `${capturePath}/${resourceID(id)}/candidates/${safeCandidateID}`,
      optionsWithSignal("PATCH", body, signal)
    ));
  },

  save(id, items, { signal } = {}) {
    const safeItems = validateSaveItems(items);
    return withCandidateIssues(() => request(
      `${capturePath}/${resourceID(id)}/save`,
      optionsWithSignal("POST", { items: safeItems }, signal)
    ));
  },

  versions(elementId, { signal } = {}) {
    const safeElementID = integerParam(elementId, "elementId");
    return request(`${elementPath}/${safeElementID}/versions`, { signal });
  },

  rollback(elementId, version, { signal } = {}) {
    const safeElementID = integerParam(elementId, "elementId");
    const safeVersion = integerParam(version, "version");
    return request(
      `${elementPath}/${safeElementID}/versions/${safeVersion}/rollback`,
      optionsWithSignal("POST", undefined, signal)
    );
  }
};
