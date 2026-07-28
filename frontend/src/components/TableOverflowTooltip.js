import { useEffect, useState } from "react";

export function TableOverflowTooltip() {
  const [tooltip, setTooltip] = useState(null);

  useEffect(() => {
    const hide = () => setTooltip(null);
    function show(event) {
      const content = event.target.closest?.("[data-overflow-tooltip]");
      if (!content || content.scrollWidth <= content.clientWidth) return hide();
      const rect = content.getBoundingClientRect();
      setTooltip({
        text: content.textContent?.trim() || "",
        left: Math.min(window.innerWidth - 196, Math.max(196, rect.left + rect.width / 2)),
        top: rect.top
      });
    }

    document.addEventListener("pointerover", show);
    document.addEventListener("pointerout", hide);
    document.addEventListener("focusin", show);
    document.addEventListener("focusout", hide);
    window.addEventListener("scroll", hide, true);
    window.addEventListener("resize", hide);
    return () => {
      document.removeEventListener("pointerover", show);
      document.removeEventListener("pointerout", hide);
      document.removeEventListener("focusin", show);
      document.removeEventListener("focusout", hide);
      window.removeEventListener("scroll", hide, true);
      window.removeEventListener("resize", hide);
    };
  }, []);

  if (!tooltip?.text) return null;
  return <div className="overflow-tooltip-bubble" role="tooltip" style={{ left: tooltip.left, top: tooltip.top }}>{tooltip.text}</div>;
}
