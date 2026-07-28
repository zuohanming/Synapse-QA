export function StateBlock({ loading, error, children }) {
  if (loading) {
    return <div aria-live="polite" className="state-block state-loading" role="status">正在加载</div>;
  }
  if (error) {
    return <div className="state-block error" role="alert">{error}</div>;
  }
  return children;
}
