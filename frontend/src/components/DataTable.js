const defaultColumnWidths = {
  select: 48,
  id: 82,
  status: 112,
  priority: 92,
  version: 112,
  updatedAt: 168,
  createdAt: 168,
  actions: 168
};

export function DataTable({ columns, rows, rowKey = "id", emptyText = "暂无数据", fitContainer = false }) {
  const tableWidth = fitContainer ? null : columns.reduce((total, column) => {
    const width = column.width || defaultColumnWidths[column.key] || 160;
    return total + (typeof width === "string" && width.endsWith("%") ? defaultColumnWidths[column.key] || 160 : Number(width));
  }, 0);
  return (
    <div className={fitContainer ? "table-wrap table-wrap-fit" : "table-wrap"}>
      <table className="data-table" data-fit-container={fitContainer ? "true" : undefined} style={fitContainer ? undefined : { minWidth: `${tableWidth}px` }}>
        <colgroup>
          {columns.map((column) => {
            const width = column.width || defaultColumnWidths[column.key] || 160;
            return <col key={column.key} style={{ width: typeof width === "string" && width.endsWith("%") ? width : `${width}px` }} />;
          })}
        </colgroup>
        <thead>
          <tr>
            {columns.map((column) => (
              <th className={column.headerClassName || column.className} key={column.key} scope="col">{column.title}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows?.length ? (
            rows.map((row) => (
              <tr key={row[rowKey]}>
                {columns.map((column) => (
                  <td className={column.className} key={column.key}>
                    <div className={["actions", "operations", "select"].includes(column.key) ? "table-cell-content table-cell-actions" : "table-cell-content"} data-overflow-tooltip={["actions", "operations", "select"].includes(column.key) ? undefined : "auto"} tabIndex={["actions", "operations", "select"].includes(column.key) ? undefined : 0}>
                      {column.render ? column.render(row) : row[column.key]}
                    </div>
                  </td>
                ))}
              </tr>
            ))
          ) : (
            <tr>
              <td className="table-empty-cell" colSpan={columns.length}>{emptyText}</td>
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
  const safeTotalPages = Math.max(1, totalPages || 1);
  const safePage = Math.min(Math.max(1, page || 1), safeTotalPages);
  const pageItems = buildPageItems(safePage, safeTotalPages);

  return (
    <nav className="pager-bar" aria-label="分页导航">
      <span className="pager-total">共 {total || 0} 条</span>
      <div className="pager-actions">
        <button className="pager-button pager-arrow" aria-label="上一页" disabled={safePage <= 1} onClick={() => onPageChange(safePage - 1)} type="button">
          <ChevronLeft size={15} />
        </button>
        <div className="pager-pages">
          {pageItems.map((item, index) =>
            item === "ellipsis" ? (
              <span className="pager-ellipsis" key={`ellipsis-${index}`}>…</span>
            ) : (
              <button
                aria-current={item === safePage ? "page" : undefined}
                aria-label={`第 ${item} 页`}
                className={item === safePage ? "pager-button pager-current" : "pager-button"}
                key={item}
                onClick={() => onPageChange(item)}
                type="button"
              >
                {item}
              </button>
            )
          )}
        </div>
        <button className="pager-button pager-arrow" aria-label="下一页" disabled={safePage >= safeTotalPages} onClick={() => onPageChange(safePage + 1)} type="button">
          <ChevronRight size={15} />
        </button>
        <select aria-label="每页条数" className="text-input pager-size" value={pageSize} onChange={(event) => onPageSizeChange(Number(event.target.value))}>
          <option value={10}>10 条/页</option>
          <option value={20}>20 条/页</option>
          <option value={50}>50 条/页</option>
        </select>
      </div>
    </nav>
  );
}

function buildPageItems(page, totalPages) {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, index) => index + 1);
  }

  const items = [1];
  const start = Math.max(2, page - 1);
  const end = Math.min(totalPages - 1, page + 1);

  if (start > 2) items.push("ellipsis");
  for (let current = start; current <= end; current += 1) items.push(current);
  if (end < totalPages - 1) items.push("ellipsis");
  items.push(totalPages);
  return items;
}
import { ChevronLeft, ChevronRight } from "lucide-react";
