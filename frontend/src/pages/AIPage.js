import { useState, useEffect, useCallback, useRef } from "react";
import { MessageSquare, CheckCircle, Wand2, AlertTriangle, Cpu, Sparkles, ArrowRight, Zap, FileText, Search, Eye, EyeOff, Check, X, Loader2, RefreshCw, Shield, Sliders, Send, RotateCcw, Bot, User, Plus, Trash2 } from "lucide-react";
import { aiService } from "../services/aiService.js";
import "./AIPage.css";

export function AIPage({ activePath }) {
  const subPage = activePath[1] || "AI 助手";

  return (
    <div className="section-stack">
      <section className="resource-panel" style={{ padding: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <div className="ai-page">
          <div className="ai-page-header">
            <div className="ai-page-title">
              <Sparkles size={22} className="ai-title-icon" />
              <h2>AI 智能</h2>
            </div>
            <p className="ai-page-desc">AI 驱动的测试辅助，提升效率，减少重复工作</p>
          </div>
          <div className="ai-page-body">
            {subPage === "AI 助手" && <AICopilot />}
            {subPage === "智能断言" && <AIAssertionPlaceholder />}
            {subPage === "用例生成" && <AICaseGenPlaceholder />}
            {subPage === "失败分析" && <AIFailureAnalysisPlaceholder />}
            {subPage === "AI 设置" && <AISettingsPlaceholder />}
          </div>
        </div>
      </section>
    </div>
  );
}

function FeatureCard({ icon, title, desc, features, status, color }) {
  return (
    <div className="ai-card">
      <div className="ai-card-header">
        <div className="ai-card-icon-wrap" style={{ background: `${color}15`, color }}>
          {icon}
        </div>
        <div className="ai-card-title-wrap">
          <h3>{title}</h3>
          <span className={`ai-card-badge ai-badge-${status === "规划中" ? "plan" : status === "开发中" ? "dev" : "done"}`}>{status}</span>
        </div>
      </div>
      <p className="ai-card-desc">{desc}</p>
      <div className="ai-card-features">
        {features.map((f, i) => (
          <div key={i} className="ai-card-feature">
            <ArrowRight size={13} />
            <span>{f}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

const AI_CHAT_HISTORY_KEY = "synapse_ai_chat_history";
const AI_MAX_HISTORY = 50;
const AI_GREETING = { role: "assistant", content: "你好！我是 Synapse QA 的 AI 助手。我可以帮您分析测试问题、提供操作建议、解答平台使用疑问。请随时向我提问。" };

function loadChatHistory() {
  try {
    const raw = localStorage.getItem(AI_CHAT_HISTORY_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed) && parsed.length > 0) return parsed;
    }
  } catch { /* ignore corrupted data */ }
  return [AI_GREETING];
}

function saveChatHistory(messages) {
  try {
    // Exclude system/tool messages from persistence
    const toSave = messages.filter(m => m.role !== "system").slice(-AI_MAX_HISTORY);
    localStorage.setItem(AI_CHAT_HISTORY_KEY, JSON.stringify(toSave));
  } catch { /* storage full or disabled */ }
}

function AICopilot() {
  const [messages, setMessages] = useState(loadChatHistory);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [streamingText, setStreamingText] = useState("");
  const [modelName, setModelName] = useState("");
  const [error, setError] = useState("");
  const [toolActivity, setToolActivity] = useState(null); // { tool, args, result, success }
  const inputRef = useRef(null);
  const bottomRef = useRef(null);

  // Persist chat history whenever messages change
  useEffect(() => {
    saveChatHistory(messages);
  }, [messages]);

  // Auto-scroll to bottom
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, streamingText]);

  const clearChat = () => {
    const reset = [AI_GREETING];
    setMessages(reset);
    localStorage.removeItem(AI_CHAT_HISTORY_KEY);
    setStreamingText("");
    setModelName("");
    setError("");
  };

  const sendMessage = async () => {
    const text = input.trim();
    if (!text || streaming) return;

    setInput("");
    setError("");
    setToolActivity(null);
    const userMsg = { role: "user", content: text };
    const newMessages = [...messages, userMsg];
    setMessages(newMessages);

    setStreaming(true);
    setStreamingText("");
    setModelName("");

    try {
      const apiMessages = newMessages.map(m => ({ role: m.role, content: m.content }));
      let full = "";
      let toolMsgs = []; // track tool call results for final message
      for await (const evt of aiService.chatStream(apiMessages)) {
        switch (evt.type) {
          case "model":
            setModelName(evt.model);
            break;
          case "tool_call":
            setToolActivity({ tool: evt.tool, args: evt.toolArgs });
            break;
          case "tool_result":
            toolMsgs.push({ tool: evt.tool, result: evt.result, success: evt.success });
            setToolActivity(prev => prev ? { ...prev, result: evt.result, success: evt.success } : null);
            // Add tool result as a system message for visibility
            setMessages(prev => [...prev, {
              role: "system",
              content: evt.result,
              tool: evt.tool,
              success: evt.success,
            }]);
            break;
          case "content":
            full += evt.content;
            setStreamingText(full);
            setToolActivity(null); // tool phase done, show streaming text
            break;
          case "done":
            break;
          case "error":
            setError(evt.error);
            break;
          default:
            // Legacy format: { model, content, done, error }
            if (evt.model) setModelName(evt.model);
            else if (evt.error) setError(evt.error);
            else if (evt.content) {
              full += evt.content;
              setStreamingText(full);
            }
        }
      }
      if (full) {
        setMessages(prev => [...prev, { role: "assistant", content: full }]);
      } else if (!error) {
        setMessages(prev => [...prev, { role: "assistant", content: "AI 返回了空内容，请重试。" }]);
      }
    } catch (e) {
      setError(e.message || "请求失败");
    } finally {
      setStreaming(false);
      setStreamingText("");
      setToolActivity(null);
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  return (
    <div className="ai-copilot">
      {/* Header */}
      <div className="ai-copilot-header">
        <div className="ai-copilot-title">
          <Bot size={18} />
          <span>AI 助手</span>
          {modelName && <span className="ai-model-badge">{modelName}</span>}
        </div>
        <button className="ai-clear-btn" onClick={clearChat} title="清空对话" type="button">
          <RotateCcw size={15} />
        </button>
      </div>

      {/* Messages */}
      <div className="ai-copilot-messages">
        {messages.map((m, i) => {
          if (m.role === "system") {
            // Tool execution result
            const success = m.success !== false;
            return (
              <div key={i} className="ai-msg tool-msg">
                <div className="ai-msg-avatar tool-avatar">
                  {success ? <Check size={12} /> : <X size={12} />}
                </div>
                <div className={`ai-msg-bubble tool-bubble ${success ? "success" : "fail"}`}>
                  <span className="tool-label">{m.tool || "工具"}：</span>
                  {m.content}
                </div>
              </div>
            );
          }
          return (
            <div key={i} className={`ai-msg ${m.role === "user" ? "user" : "bot"}`}>
              <div className="ai-msg-avatar">
                {m.role === "user" ? <User size={14} /> : <Bot size={14} />}
              </div>
              <div className="ai-msg-bubble">{m.content}</div>
            </div>
          );
        })}

        {/* Tool activity indicator (during execution) */}
        {toolActivity && !toolActivity.result && (
          <div className="ai-msg tool-msg">
            <div className="ai-msg-avatar tool-avatar running">
              <Loader2 size={12} className="ai-spin" />
            </div>
            <div className="ai-msg-bubble tool-bubble running">
              <span className="tool-label">{toolActivity.tool}：</span>
              正在执行...
            </div>
          </div>
        )}

        {/* Streaming message */}
        {streaming && streamingText && (
          <div className="ai-msg bot">
            <div className="ai-msg-avatar"><Bot size={14} /></div>
            <div className="ai-msg-bubble">{streamingText}<span className="ai-cursor" /></div>
          </div>
        )}

        {streaming && !streamingText && (
          <div className="ai-msg bot">
            <div className="ai-msg-avatar"><Bot size={14} /></div>
            <div className="ai-msg-bubble ai-typing"><span /><span /><span /></div>
          </div>
        )}

        {error && (
          <div className="ai-msg bot error">
            <div className="ai-msg-bubble">{error}</div>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="ai-copilot-input">
        <input
          ref={inputRef}
          className="text-input ai-chat-input"
          placeholder="输入消息，Enter 发送..."
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={streaming}
        />
        <button
          className="primary-button ai-send-btn"
          onClick={sendMessage}
          disabled={!input.trim() || streaming}
          title="发送"
          type="button"
        >
          {streaming ? <Loader2 size={16} className="ai-spin" /> : <Send size={16} />}
        </button>
      </div>
    </div>
  );
}

function AICopilotPlaceholder() { return null; }

function AIAssertionPlaceholder() {
  return (
    <div className="ai-sub-page">
      <FeatureCard
        icon={<CheckCircle size={24} />}
        title="智能断言建议"
        desc="调试接口后，AI 自动分析响应数据，推荐合适的断言规则，一键应用到测试用例。"
        status="规划中"
        color="#10b981"
        features={[
          "响应分析：自动识别字段类型和取值范围",
          "断言推荐：状态码、JSONPath、Header 断言",
          "历史学习：基于历史执行数据优化建议",
          "批量采纳：一键应用全部推荐断言",
          "置信度标注：标注每条建议的可靠性"
        ]}
      />
      <div className="ai-mock-code">
        <div className="ai-mock-code-header">响应数据</div>
        <pre>{`{
  "code": 0,
  "data": {
    "userId": 10001,
    "token": "eyJhbGci...",
    "expiresIn": 7200
  }
}`}</pre>
        <div className="ai-mock-code-header" style={{ marginTop: 16 }}>AI 建议断言</div>
        <pre className="ai-mock-suggestions">{`✅ $.code == 0              ← 业务状态码
✅ $.data.userId > 0          ← ID 应为正整数
✅ $.data.token 格式为 JWT     ← 三段 Base64
✅ $.data.expiresIn ≤ 86400   ← 合理有效期
💡 建议追加 Header 断言        ← Set-Cookie`}</pre>
      </div>
    </div>
  );
}

function AICaseGenPlaceholder() {
  return (
    <div className="ai-sub-page">
      <FeatureCard
        icon={<Wand2 size={24} />}
        title="用例生成"
        desc="基于接口文档、需求描述或 Swagger 定义，AI 自动生成全面的测试用例。"
        status="规划中"
        color="#f59e0b"
        features={[
          "接口文档 → 用例：粘贴 Swagger/接口定义，生成用例",
          "需求描述 → 用例：自然语言描述转测试场景",
          "参数组合：自动 Pairwise 参数组合覆盖",
          "异常场景：自动生成边界值、空值、类型异常用例",
          "一键导入：生成后直接导入到用例库"
        ]}
      />
    </div>
  );
}

function AIFailureAnalysisPlaceholder() {
  return (
    <div className="ai-sub-page">
      <FeatureCard
        icon={<AlertTriangle size={24} />}
        title="失败智能分析"
        desc="测试执行失败后，AI 自动采集上下文信息，诊断根因并给出修复建议。"
        status="规划中"
        color="#ef4444"
        features={[
          "自动采集：收集请求/响应/日志/历史记录",
          "根因分析：按概率排序的可能原因",
          "历史对比：对比最近一次通过时的差异",
          "关联分析：检测同模块其他用例是否也失败",
          "修复建议：给出具体的修复步骤"
        ]}
      />
    </div>
  );
}

const PROVIDER_OPTIONS = [
  { value: "openai", label: "OpenAI", color: "#10a37f", defaultBase: "https://api.openai.com/v1" },
  { value: "deepseek", label: "DeepSeek", color: "#4f46e5", defaultBase: "https://api.deepseek.com" },
  { value: "anthropic", label: "Anthropic", color: "#d97757", defaultBase: "https://api.anthropic.com" },
  { value: "gemini", label: "Google Gemini", color: "#4285f4", defaultBase: "https://generativelanguage.googleapis.com" },
  { value: "openrouter", label: "OpenRouter", color: "#6366f1", defaultBase: "https://openrouter.ai/api/v1" },
];

function getProviderMeta(key) {
  return PROVIDER_OPTIONS.find(p => p.value === key) || { value: key, label: key, color: "#6b7280", defaultBase: "" };
}

function AISettings() {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savingPref, setSavingPref] = useState(null);
  const [testResult, setTestResult] = useState(null);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState(null); // null = create, string = edit
  const [form, setForm] = useState({});
  const [showKey, setShowKey] = useState(false);
  const [error, setError] = useState("");

  const loadConfig = useCallback(async () => {
    try {
      setLoading(true);
      setError("");
      const data = await aiService.getConfig();
      setConfig(data);
    } catch (e) {
      setError(e.message || "加载配置失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadConfig(); }, [loadConfig]);

  const openCreate = () => {
    setEditingId(null);
    setForm({ id: "", provider: "openai", name: "", label: "", apiKey: "", baseUrl: "", enabled: true });
    setShowKey(false);
    setTestResult(null);
    setShowForm(true);
  };

  const openEdit = (model) => {
    setEditingId(model.id);
    setForm({ ...model });
    setShowKey(false);
    setTestResult(null);
    setShowForm(true);
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingId(null);
    setForm({});
    setTestResult(null);
  };

  const handleFormChange = (field, value) => {
    setForm(f => {
      const next = { ...f, [field]: value };
      // Auto-generate ID from provider + name
      if ((field === "provider" || field === "name") && !editingId) {
        next.id = `${next.provider || ""}-${next.name || ""}`.replace(/\s+/g, "-").toLowerCase();
      }
      // Auto-fill default base URL when provider changes
      if (field === "provider") {
        const meta = getProviderMeta(value);
        if (!f.baseUrl || PROVIDER_OPTIONS.some(p => p.defaultBase === f.baseUrl)) {
          next.baseUrl = meta.defaultBase;
        }
      }
      return next;
    });
  };

  const handleSave = async () => {
    try {
      setSaving(true);
      setError("");
      const model = {
        id: form.id,
        provider: form.provider,
        name: form.name.trim(),
        label: form.label?.trim() || form.name.trim(),
        apiKey: form.apiKey,
        baseUrl: form.baseUrl || undefined,
        enabled: form.enabled,
      };
      const data = await aiService.saveModel(model);
      setConfig(data);
      closeForm();
    } catch (e) {
      setError(e.message || "保存失败");
    } finally {
      setSaving(false);
    }
  };

  const handleTestAndSave = async () => {
    try {
      setSaving(true);
      setTestResult(null);
      setError("");

      const result = await aiService.testConnection(form.provider, form.apiKey, form.baseUrl);
      setTestResult(result);

      if (!result.success) {
        setSaving(false);
        return;
      }

      // Auto-save on test success
      const model = {
        id: form.id,
        provider: form.provider,
        name: form.name.trim(),
        label: form.label?.trim() || form.name.trim(),
        apiKey: form.apiKey,
        baseUrl: form.baseUrl || undefined,
        enabled: true,
      };
      const data = await aiService.saveModel(model);
      setConfig(data);
      setForm(f => ({ ...f, enabled: true }));
    } catch (e) {
      setError(e.message || "保存失败");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id) => {
    if (!window.confirm(`确定要删除模型「${id}」吗？`)) return;
    try {
      setError("");
      const data = await aiService.deleteModel(id);
      setConfig(data);
    } catch (e) {
      setError(e.message || "删除失败");
    }
  };

  const prefChange = async (key, value) => {
    try {
      setSavingPref(key);
      const newPrefs = { ...config.preferences, [key]: value };
      const data = await aiService.savePreferences(newPrefs);
      setConfig(data);
    } catch (e) {
      setError(e.message || "保存偏好失败");
    } finally {
      setSavingPref(null);
    }
  };

  if (loading) {
    return (
      <div className="ai-sub-page">
        <div className="ai-loading"><Loader2 size={24} className="ai-spin" /><span>加载 AI 配置...</span></div>
      </div>
    );
  }

  if (error && !config) {
    return (
      <div className="ai-sub-page">
        <div className="ai-error">
          <AlertTriangle size={20} /><span>{error}</span>
          <button className="primary-button" onClick={loadConfig}>重试</button>
        </div>
      </div>
    );
  }

  const models = config?.models || [];
  const enabledCount = models.filter(m => m.enabled && m.apiKey).length;

  // Build model options for preferences dropdown
  const prefOptions = models
    .filter(m => m.enabled && m.apiKey)
    .map(m => {
      const meta = getProviderMeta(m.provider);
      return {
        value: m.id,
        label: `${meta.label} · ${m.label || m.name}`,
      };
    });

  return (
    <div className="ai-sub-page">
      {error && <div className="ai-toast-error"><AlertTriangle size={15} /><span>{error}</span><button onClick={() => setError("")}><X size={14} /></button></div>}

      {/* 总览卡片 */}
      <div className="ai-settings-overview">
        <div className="ai-overview-stat">
          <div className="ai-overview-icon" style={{ background: "#8b5cf618", color: "#8b5cf6" }}><Cpu size={20} /></div>
          <div><strong>{enabledCount}</strong><span>已启用模型</span></div>
        </div>
        <div className="ai-overview-stat">
          <div className="ai-overview-icon" style={{ background: "#f59e0b18", color: "#f59e0b" }}><Zap size={20} /></div>
          <div><strong>{config?.security?.rateLimitPerMinute || 0}</strong><span>次/分钟限流</span></div>
        </div>
        <div className="ai-overview-stat">
          <div className="ai-overview-icon" style={{ background: "#10b98118", color: "#10b981" }}><Shield size={20} /></div>
          <div><strong>{config?.security?.dataMasking ? "已启用" : "未启用"}</strong><span>数据脱敏</span></div>
        </div>
      </div>

      {/* 模型管理区域 */}
      <div className="ai-section">
        <div className="ai-section-header-row">
          <div>
            <h3 className="ai-section-title"><Cpu size={16} />模型管理</h3>
            <p className="ai-section-desc">新建和管理 AI 模型配置</p>
          </div>
          <button className="primary-button ai-new-model-btn" onClick={openCreate} type="button">
            <Plus size={15} />新建模型
          </button>
        </div>

        {models.length === 0 ? (
          <div className="ai-model-empty-state">
            <Cpu size={32} style={{ color: "#ccc" }} />
            <p>暂无模型配置</p>
            <span>点击"新建模型"添加您的第一个 AI 模型</span>
          </div>
        ) : (
          <div className="ai-model-grid">
            {models.map(m => {
              const meta = getProviderMeta(m.provider);
              const isEnabled = m.enabled && m.apiKey;
              return (
                <div key={m.id} className={`ai-model-card ${isEnabled ? "enabled" : ""}`}>
                  <div className="ai-model-card-top">
                    <div className="ai-model-card-provider" style={{ background: `${meta.color}18`, color: meta.color }}>
                      {meta.label}
                    </div>
                    <div className="ai-model-card-actions">
                      <button className="icon-text-button" onClick={() => openEdit(m)} title="编辑" type="button">
                        <Sliders size={13} />
                      </button>
                      <button className="icon-text-button ai-model-del-btn" onClick={() => handleDelete(m.id)} title="删除" type="button">
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </div>
                  <div className="ai-model-card-body">
                    <h4 className="ai-model-card-name">{m.label || m.name}</h4>
                    <span className="ai-model-card-id">{m.id}</span>
                  </div>
                  <div className="ai-model-card-footer">
                    <span className={`ai-status-dot ${isEnabled ? "on" : "off"}`} />
                    <span className="ai-status-text">{isEnabled ? "已启用" : isEnabled === false ? "已禁用" : "未配置密钥"}</span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* 新建/编辑弹窗 */}
      {showForm && (() => {
        const meta = getProviderMeta(form.provider);
        return (
          <div className="modal-backdrop" onClick={closeForm}>
            <div className="modal-card ai-provider-modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 540 }}>
              <div className="modal-header">
                <strong>{editingId ? `编辑模型 · ${editingId}` : "新建模型"}</strong>
                <button className="modal-close" onClick={closeForm}><X size={18} /></button>
              </div>
              <div className="modal-body" style={{ padding: "20px", display: "grid", gap: 16 }}>
                {/* 服务商 */}
                <div className="form-field">
                  <span>服务商</span>
                  <select
                    className="text-input"
                    value={form.provider}
                    onChange={e => handleFormChange("provider", e.target.value)}
                  >
                    {PROVIDER_OPTIONS.map(p => (
                      <option key={p.value} value={p.value}>{p.label}</option>
                    ))}
                  </select>
                </div>

                {/* 模型名称 */}
                <div className="form-field">
                  <span>模型名称</span>
                  <input
                    className="text-input"
                    value={form.name}
                    onChange={e => handleFormChange("name", e.target.value)}
                    placeholder="如 deepseek-chat、gpt-4o"
                  />
                </div>

                {/* 显示名称 */}
                <div className="form-field">
                  <span>显示名称（可选）</span>
                  <input
                    className="text-input"
                    value={form.label}
                    onChange={e => handleFormChange("label", e.target.value)}
                    placeholder="给模型起个好记的名字"
                  />
                </div>

                {/* API Key */}
                <div className="form-field">
                  <span>API Key</span>
                  <div className="ai-input-group">
                    <input
                      className="text-input"
                      type={showKey ? "text" : "password"}
                      value={form.apiKey}
                      onChange={e => handleFormChange("apiKey", e.target.value)}
                      placeholder="sk-..."
                      style={{ flex: 1 }}
                    />
                    <button className="ai-key-toggle" onClick={() => setShowKey(v => !v)} type="button">
                      {showKey ? <EyeOff size={15} /> : <Eye size={15} />}
                    </button>
                  </div>
                </div>

                {/* Base URL */}
                <div className="form-field">
                  <span>Base URL（可选）</span>
                  <input
                    className="text-input"
                    value={form.baseUrl}
                    onChange={e => handleFormChange("baseUrl", e.target.value)}
                    placeholder={meta.defaultBase || "默认地址"}
                  />
                </div>

                {/* Model ID */}
                <div className="form-field">
                  <span>模型标识 ID</span>
                  <input
                    className="text-input"
                    value={form.id}
                    onChange={e => handleFormChange("id", e.target.value)}
                    placeholder="自动生成"
                    disabled={!!editingId}
                    style={{ opacity: editingId ? 0.6 : 1 }}
                  />
                </div>

                {/* 启用开关 */}
                <label className="ai-toggle-row">
                  <span>启用此模型</span>
                  <button
                    className={`ai-toggle ${form.enabled ? "on" : ""}`}
                    onClick={() => handleFormChange("enabled", !form.enabled)}
                    type="button"
                  >
                    <span className="ai-toggle-thumb" />
                  </button>
                </label>

                {/* 测试结果 */}
                {testResult && (
                  <div className={`ai-test-result ${testResult.success ? "success" : "fail"}`}>
                    {testResult.success ? <Check size={15} /> : <X size={15} />}
                    <span>{testResult.message}</span>
                    {testResult.latency != null && <span className="ai-latency">{testResult.latency}ms</span>}
                  </div>
                )}

                {/* 操作按钮 */}
                <div className="modal-actions" style={{ display: "flex", gap: 8, justifyContent: "flex-end", margin: 0, padding: 0, background: "none", border: 0 }}>
                  <button className="icon-text-button" onClick={handleSave} disabled={!form.name.trim() || saving}>
                    {saving ? <Loader2 size={14} className="ai-spin" /> : null}
                    仅保存
                  </button>
                  <button className="primary-button" onClick={handleTestAndSave} disabled={!form.apiKey || !form.name.trim() || saving}>
                    {saving ? <Loader2 size={14} className="ai-spin" /> : <RefreshCw size={14} />}
                    测试并保存
                  </button>
                </div>
              </div>
            </div>
          </div>
        );
      })()}

      {/* 场景偏好 */}
      <div className="ai-section">
        <h3 className="ai-section-title"><Sliders size={16} />场景偏好</h3>
        <p className="ai-section-desc">为不同 AI 功能指定默认使用的模型</p>
        {prefOptions.length === 0 ? (
          <p className="ai-pref-empty">请先新建并启用至少一个模型</p>
        ) : (
          <div className="ai-pref-grid">
            {[
              { key: "copilot", label: "AI 助手", icon: <MessageSquare size={16} /> },
              { key: "assertions", label: "智能断言", icon: <CheckCircle size={16} /> },
              { key: "generation", label: "用例生成", icon: <Wand2 size={16} /> },
              { key: "analysis", label: "失败分析", icon: <AlertTriangle size={16} /> },
            ].map(item => (
              <div key={item.key} className="ai-pref-row">
                <span className="ai-pref-label">{item.icon}{item.label}</span>
                <div className="ai-pref-select-wrap">
                  {savingPref === item.key && <Loader2 size={13} className="ai-spin" />}
                  <select
                    className="text-input ai-pref-select"
                    value={config?.preferences?.[item.key] || ""}
                    onChange={e => prefChange(item.key, e.target.value)}
                  >
                    <option value="">— 未选择 —</option>
                    {prefOptions.map(opt => (
                      <option key={opt.value} value={opt.value}>{opt.label}</option>
                    ))}
                  </select>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 安全策略 */}
      <div className="ai-section">
        <h3 className="ai-section-title"><Shield size={16} />安全策略</h3>
        <p className="ai-section-desc">控制 AI 请求的安全性和成本</p>
        <div className="ai-security-grid">
          <div className="ai-security-item">
            <span>数据脱敏</span>
            <span className={`ai-security-badge ${config?.security?.dataMasking ? "on" : "off"}`}>
              {config?.security?.dataMasking ? "已启用" : "已禁用"}
            </span>
          </div>
          <div className="ai-security-item">
            <span>单次最大 Token</span>
            <strong>{config?.security?.maxTokensPerRequest?.toLocaleString() || "—"}</strong>
          </div>
          <div className="ai-security-item">
            <span>速率限制</span>
            <strong>{config?.security?.rateLimitPerMinute || "—"} 次/分钟</strong>
          </div>
        </div>
      </div>
    </div>
  );
}

const AISettingsPlaceholder = AISettings;
