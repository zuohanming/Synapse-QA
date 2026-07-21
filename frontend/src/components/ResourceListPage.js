import { useMemo, useState } from "react";
import { DataTable, PaginationBar, TablePanel } from "./DataTable.js";
import { PageHeader } from "./PageHeader.js";
import { StateBlock } from "./StateBlock.js";

export function ResourceListPage({ title, description, panelTitle, rows = [], columns, loading, error }) {
  const [keyword, setKeyword] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const filteredRows = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return rows;
    return rows.filter((row) =>
      columns.some((column) => String(row[column.key] ?? "").toLowerCase().includes(normalized))
    );
  }, [columns, query, rows]);

  const totalPages = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const pageRows = filteredRows.slice((page - 1) * pageSize, page * pageSize);

  function handleSearch(event) {
    event.preventDefault();
    setPage(1);
    setQuery(keyword);
  }

  function handleReset() {
    setKeyword("");
    setQuery("");
    setPage(1);
  }

  return (
    <div className="section-stack">
      <PageHeader title={title} description={description} />
      <section className="resource-panel">
        <div className="panel-header"><strong>{panelTitle}</strong></div>
        <form className="filter-grid" onSubmit={handleSearch}>
          <label className="form-field simple-list-search">
            <span>关键字</span>
            <input className="text-input" placeholder={`搜索${panelTitle}`} value={keyword} onChange={(event) => setKeyword(event.target.value)} />
          </label>
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="form-field form-field-placeholder" />
          <div className="toolbar-row">
            <button className="primary-button compact-button" type="submit">搜索</button>
            <button className="icon-text-button compact-button" onClick={handleReset} type="button">重置</button>
          </div>
        </form>
        <StateBlock loading={loading} error={error}>
          <TablePanel>
            <DataTable rows={pageRows} columns={columns} emptyText={`暂无${panelTitle}数据`} />
            <PaginationBar
              page={page}
              pageSize={pageSize}
              total={filteredRows.length}
              totalPages={totalPages}
              onPageChange={setPage}
              onPageSizeChange={(value) => { setPage(1); setPageSize(value); }}
            />
          </TablePanel>
        </StateBlock>
      </section>
    </div>
  );
}
