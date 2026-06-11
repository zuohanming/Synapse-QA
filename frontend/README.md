# Synapse QA Frontend

React 前端采用以下结构：

```text
public/                 静态资源目录
src/                    源代码目录
  components/           公共组件
  pages/                页面组件
  hooks/                自定义 Hooks
  utils/                工具函数
  assets/               资源文件
  services/             API 服务
  context/              上下文
  redux/                状态管理预留
  routes/               路由配置
  config/               配置文件
  App.js                根组件
  index.js              入口文件
```

## 启动

```powershell
npm install
npm run dev
```

默认访问地址：

```text
http://127.0.0.1:4173/
```

后端 API 地址默认读取 `VITE_API_BASE`，未配置时使用：

```text
http://127.0.0.1:8080/api
```
