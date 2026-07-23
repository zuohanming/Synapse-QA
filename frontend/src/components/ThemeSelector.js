import { useState, useEffect } from "react";

export function ThemeSelector({ onThemeSelect }) {
  const [selected, setSelected] = useState(null);

  const themes = [
    { id: "blue", name: "科技蓝渐变", desc: "现代企业风格，渐变蓝色主色调" },
    { id: "dark", name: "暗色主题", desc: "护眼深色模式，霓虹配色点缀" },
    { id: "fresh", name: "简约清新", desc: "浅色系设计，清爽视觉体验" }
  ];

  useEffect(() => {
    // 默认为科技蓝主题
    if (!selected) {
      setSelected("blue");
      if (onThemeSelect) onThemeSelect("blue");
    }
  }, [selected, onThemeSelect]);

  const handleSelect = (themeId) => {
    setSelected(themeId);
    if (onThemeSelect) onThemeSelect(themeId);
  };

  return (
    <div style={{
      position: "fixed",
      top: "50%",
      left: "50%",
      transform: "translate(-50%, -50%)",
      background: "white",
      padding: "40px",
      borderRadius: "16px",
      boxShadow: "0 20px 40px rgba(0,0,0,0.2)",
      zIndex: 9999,
      maxWidth: "600px",
      width: "90%"
    }}>
      <h1 style={{
        margin: "0 0 8px",
        fontSize: "28px",
        fontWeight: "700",
        background: "linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)",
        WebkitBackgroundClip: "text",
        WebkitTextFillColor: "transparent",
        textAlign: "center"
      }}>
        Synapse QA 主题选择
      </h1>
      <p style={{
        margin: "0 0 32px",
        color: "#64748b",
        textAlign: "center",
        fontSize: "15px"
      }}>
        选择您喜欢的界面风格
      </p>

      <div style={{
        display: "grid",
        gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))",
        gap: "16px"
      }}>
        {themes.map((theme) => (
          <button
            key={theme.id}
            onClick={() => handleSelect(theme.id)}
            style={{
              padding: "24px 20px",
              border: selected === theme.id
                ? "2px solid #3b82f6"
                : "2px solid #e2e8f0",
              borderRadius: "12px",
              background: selected === theme.id
                ? "linear-gradient(135deg, rgba(59, 130, 246, 0.1) 0%, rgba(139, 92, 246, 0.1) 100%)"
                : "white",
              cursor: "pointer",
              transition: "all 0.2s ease",
              textAlign: "left"
            }}
            onMouseEnter={(e) => {
              if (selected !== theme.id) {
                e.currentTarget.style.borderColor = "#93c5fd";
                e.currentTarget.style.background = "rgba(59, 130, 246, 0.05)";
              }
            }}
            onMouseLeave={(e) => {
              if (selected !== theme.id) {
                e.currentTarget.style.borderColor = "#e2e8f0";
                e.currentTarget.style.background = "white";
              }
            }}
          >
            <div style={{
              fontSize: "16px",
              fontWeight: "600",
              color: selected === theme.id ? "#3b82f6" : "#0f172a",
              marginBottom: "6px"
            }}>
              {theme.name}
            </div>
            <div style={{
              fontSize: "13px",
              color: "#64748b",
              lineHeight: "1.4"
            }}>
              {theme.desc}
            </div>
          </button>
        ))}
      </div>

      <div style={{
        marginTop: "32px",
        textAlign: "center",
        fontSize: "13px",
        color: "#94a3b8"
      }}>
        <p>💡 提示：您可以随时在系统设置中切换主题</p>
      </div>
    </div>
  );
}
