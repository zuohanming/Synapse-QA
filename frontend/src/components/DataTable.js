export function DataTable({ columns, rows, rowKey = "id", emptyText = "暂无数据" }) {
  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column.key}>{column.title}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows?.length ? (
            rows.map((row) => (
              <tr key={row[rowKey]}>
                {columns.map((column) => (
                  <td key={column.key}>{column.render ? column.render(row) : row[column.key]}</td>
                ))}
              </tr>
            ))
          ) : (
            <tr>
              <td colSpan={columns.length}>{emptyText}</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

export function TablePanel({ children, compact = false }) {
  return <div className={compact ? "table-panel table-panel-compact" : "table-panel"}>{children}</div>;
}

export function PaginationBar({ page, pageSize, total, totalPages, onPageChange, onPageSizeChange }) {
  return (
    <div className="pager-bar">
      <div className="pager-actions">
        <span className="pager-total">共 {total} 条</span>
        <button className="icon-text-button compact-button" disabled={page <= 1} onClick={() => onPageChange(page - 1)} type="button">
          上一页
        </button>
        <span className="pager-current">{page}</span>
        <button className="icon-text-button compact-button" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)} type="button">
          下一页
        </button>
        <select className="text-input pager-size" value={pageSize} onChange={(event) => onPageSizeChange(Number(event.target.value))}>
          <option value={10}>10 条/页</option>
          <option value={20}>20 条/页</option>
          <option value={50}>50 条/页</option>
        </select>
      </div>
    </div>
  );
}
