import { useState, useEffect, useCallback } from "react";
import { Copy, Check, RefreshCw, Sparkles, Search, X, Hash, Phone, Mail, IdCard, Fingerprint, HashIcon, Calendar, Type, ArrowRight, MapPin, Building2, CreditCard, Globe, Wifi, Image, Palette, ToggleLeft, Layers, User } from "lucide-react";
import { dataFactoryService } from "../services/dataFactoryService.js";

const catIcons = {
  "{{name}}": <User size={18} />,
  "{{phone}}": <Phone size={18} />,
  "{{email}}": <Mail size={18} />,
  "{{id_card}}": <IdCard size={18} />,
  "{{uuid}}": <Fingerprint size={18} />,
  "{{int(1,100)}}": <Hash size={18} />,
  "{{float(0,100,2)}}": <HashIcon size={18} />,
  "{{timestamp}}": <Calendar size={18} />,
  "{{date(\"Y-m-d\")}}": <Calendar size={18} />,
  "{{lorem(30)}}": <Type size={18} />,
  "{{seq(\"ORD\",1,1)}}": <ArrowRight size={18} />,
  "{{enum(\"A\",\"B\",\"C\")}}": <Layers size={18} />,
  "{{nickname}}": <Sparkles size={18} />,
  "{{address}}": <MapPin size={18} />,
  "{{company}}": <Building2 size={18} />,
  "{{bank_card}}": <CreditCard size={18} />,
  "{{url}}": <Globe size={18} />,
  "{{ip}}": <Wifi size={18} />,
  "{{img(200,300)}}": <Image size={18} />,
  "{{color}}": <Palette size={18} />,
  "{{bool}}": <ToggleLeft size={18} />,
  "{{int_seq(\"key\",0,1)}}": <Hash size={18} />,
  "{{person}}": <User size={18} />,
};

export function DataFactoryPage({ activePath }) {
  const subPage = activePath[1] || "Mock 生成器";

  return (
    <div className="section-stack">
      <section className="resource-panel" style={{ padding: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {subPage === "Mock 生成器" ? <MockGeneratorPage /> : <MockGeneratorPage />}
      </section>
    </div>
  );
}

function GeneratorCard({ generator, onCopy, onPreview, previewResults, loading, active, onToggle }) {
  const [localCopied, setLocalCopied] = useState(false);

  const handleCopy = (e) => {
    e.stopPropagation();
    navigator.clipboard.writeText(generator.placeholder);
    setLocalCopied(true);
    setTimeout(() => setLocalCopied(false), 2000);
    onCopy?.(generator.placeholder);
  };

  const handlePreview = (e) => {
    e.stopPropagation();
    onPreview(generator.placeholder);
    onToggle();
  };

  const icon = catIcons[generator.placeholder] || <Sparkles size={18} />;

  return (
    <div className={`df-card ${active ? "df-card-active" : ""}`} onClick={onToggle}>
      <div className="df-card-top">
        <div className="df-card-icon">
          {icon}
        </div>
        <div className="df-card-info">
          <div className="df-card-header">
            <span className="df-card-name">{generator.name}</span>
            <span className="df-card-cat">{generator.categoryCn}</span>
          </div>
          <p className="df-card-desc">{generator.description}</p>
          <code className="df-card-syntax">{generator.placeholder}</code>
        </div>
      </div>

      {active && (
        <div className="df-card-expand" onClick={e => e.stopPropagation()}>
          <div className="df-card-divider" />
          <div className="df-card-body">
            <div className="df-card-example">
              <span className="df-card-label">示例</span>
              <code>{generator.example}</code>
            </div>

            <div className="df-card-preview">
              <div className="df-card-preview-header">
                <span className="df-card-label">预览</span>
                <button
                  className="btn compact-button"
                  onClick={handlePreview}
                  disabled={loading}
                  style={{ padding: "3px 10px", fontSize: 11, height: 26 }}
                >
                  <RefreshCw size={12} className={loading ? "spinning" : ""} />
                  刷新
                </button>
              </div>
              <div className="df-card-preview-list">
                {previewResults.map((r, i) => (
                  <span key={i} className="df-card-preview-item">{r}</span>
                ))}
              </div>
            </div>
          </div>
          <div className="df-card-actions">
            <button className="btn compact-button" onClick={handleCopy}>
              {localCopied ? <><Check size={13} /> 已复制</> : <><Copy size={13} /> 复制语法</>}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function MockGeneratorPage() {
  const [generators, setGenerators] = useState([]);
  const [search, setSearch] = useState("");
  const [expandedCard, setExpandedCard] = useState(null);
  const [previewCache, setPreviewCache] = useState({});
  const [loadingPreview, setLoadingPreview] = useState("");

  useEffect(() => {
    dataFactoryService.listGenerators().then(res => {
      if (res?.data?.generators) {
        setGenerators(res.data.generators);
      }
    });
  }, []);

  const loadPreview = useCallback(async (placeholder) => {
    if (previewCache[placeholder]) return;
    setLoadingPreview(placeholder);
    const res = await dataFactoryService.preview(placeholder, 5);
    if (res?.data?.results) {
      setPreviewCache(prev => ({ ...prev, [placeholder]: res.data.results }));
    }
    setLoadingPreview("");
  }, [previewCache]);

  const handleToggle = (placeholder) => {
    if (expandedCard === placeholder) {
      setExpandedCard(null);
    } else {
      setExpandedCard(placeholder);
      if (!previewCache[placeholder]) {
        loadPreview(placeholder);
      }
    }
  };

  const filterFn = (g) => {
    if (!search) return true;
    const s = search.toLowerCase();
    return g.name.includes(s) || g.placeholder.includes(s) || g.description.includes(s) || g.categoryCn.includes(s);
  };

  const categories = ["basic", "business", "advanced", "scene"];
  const catCN = { basic: "基础", business: "业务", advanced: "进阶", scene: "场景" };
  const catDesc = { basic: "最常用的基础数据类型", business: "业务场景专用数据", advanced: "高级生成工具", scene: "快捷场景模板" };
  const grouped = {};
  categories.forEach(cat => {
    grouped[cat] = generators.filter(g => g.category === cat && filterFn(g));
  });

  const hasResults = Object.values(grouped).some(arr => arr.length > 0);

  return (
    <div className="df-cards-page">
      <div className="df-cards-header">
        <h2><Sparkles size={22} /> Mock 生成器</h2>
        <div className="df-cards-search-wrap">
          <Search size={15} className="df-cards-search-icon" />
          <input
            type="text"
            placeholder="搜索生成器..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="df-cards-search"
          />
          {search && (
            <button className="df-cards-search-clear" onClick={() => setSearch("")}><X size={14} /></button>
          )}
        </div>
      </div>

      <div className="df-cards-body">
        {!hasResults ? (
          <div className="df-empty">
            <Sparkles size={48} strokeWidth={1} />
            <p>没有匹配的生成器</p>
          </div>
        ) : (
          categories.map(cat => {
            const items = grouped[cat] || [];
            if (items.length === 0) return null;
            return (
              <div key={cat} className="df-cards-section">
                <div className="df-cards-section-header">
                  <span className="df-cards-section-name">{catCN[cat]}</span>
                  <span className="df-cards-section-count">{items.length}</span>
                  <span className="df-cards-section-desc">{catDesc[cat]}</span>
                </div>
                <div className="df-cards-grid">
                  {items.map(g => (
                    <GeneratorCard
                      key={g.placeholder}
                      generator={g}
                      active={expandedCard === g.placeholder}
                      onToggle={() => handleToggle(g.placeholder)}
                      onPreview={loadPreview}
                      previewResults={previewCache[g.placeholder] || []}
                      loading={loadingPreview === g.placeholder}
                    />
                  ))}
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
