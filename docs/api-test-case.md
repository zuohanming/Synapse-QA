# 测试用例模块 API 文档

## 认证

所有 `/api/test-cases` 接口都需要 `Authorization: Bearer <token>`。

## 分页查询测试用例

```http
GET /api/test-cases?page=1&pageSize=20&name=登录&productId=1&status=active
```

查询参数：

- `id`
- `name`
- `productId`
- `moduleId`
- `caseType`
- `priority`
- `status`
- `owner`
- `page`
- `pageSize`

## 查询详情

```http
GET /api/test-cases/{id}
```

返回测试用例基础信息、步骤关联和参数化数据集。

## 创建测试用例

```http
POST /api/test-cases
Content-Type: application/json

{
  "productId": 1,
  "moduleId": 2,
  "pageId": 3,
  "name": "登录成功",
  "caseType": "ui",
  "priority": "P1",
  "status": "active",
  "owner": "admin",
  "tags": "smoke,login",
  "description": "验证登录成功",
  "preconditions": "存在有效账号",
  "expectedResult": "进入首页",
  "dataEnabled": true,
  "stepIds": [10, 11]
}
```

校验规则：

- `productId` 必填且产品必须存在。
- `name` 必填，最长 120 个字符。
- `caseType` 允许：`ui`、`api`、`unit`、`mixed`。
- `priority` 允许：`P0`、`P1`、`P2`、`P3`。
- `status` 允许：`draft`、`active`、`disabled`。
- `stepIds` 必须指向未删除的 `page_step`。

## 更新测试用例

```http
PATCH /api/test-cases/{id}
```

请求体同创建接口。

## 删除测试用例

```http
DELETE /api/test-cases/{id}
```

软删除测试用例。

## 导入测试用例

```http
POST /api/test-cases/import
Content-Type: application/json

{
  "items": [
    {
      "productId": 1,
      "moduleId": 2,
      "pageId": 3,
      "name": "登录成功",
      "caseType": "ui",
      "priority": "P1",
      "status": "active",
      "owner": "admin",
      "stepIds": [10, 11]
    }
  ]
}
```

单次最多导入 200 条。导入时逐条复用创建接口校验规则。

## 导出测试用例

```http
GET /api/test-cases/export?name=登录&productId=1&status=active
```

查询参数同分页查询接口，返回当前筛选条件下最多 10000 条测试用例。

## 查询参数化数据

```http
GET /api/test-cases/{id}/datasets
```

## 创建参数化数据

```http
POST /api/test-cases/{id}/datasets
Content-Type: application/json

{
  "name": "默认数据",
  "variables": {
    "username": "admin",
    "password": "admin123"
  },
  "enabled": true
}
```

## 更新参数化数据

```http
PATCH /api/test-cases/{id}/datasets/{datasetId}
```

## 删除参数化数据

```http
DELETE /api/test-cases/{id}/datasets/{datasetId}
```
