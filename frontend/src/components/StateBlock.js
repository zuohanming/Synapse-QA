export function StateBlock({ loading, error, children }) {
  if (loading) {
    return <div className="state-block">正在加载</div>;
  }
  if (error) {
    return <div className="state-block error">{error}</div>;
  }
  return children;
}
