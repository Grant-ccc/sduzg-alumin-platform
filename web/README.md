# 山大政管学院校友平台前端

一期前端采用 React + Vite + TypeScript，按技术方案预置路由、权限守卫、API 封装、状态管理、管理后台表格表单和数据大屏页面。

## 本地启动

```bash
npm install
npm run dev
```

默认开发服务地址：

```text
http://127.0.0.1:5173
```

开发环境下 `/api` 会代理到 `http://127.0.0.1:8080`，接口基础路径为 `/api/v1`。

## 登录态与权限

localStorage 只保存访问 token，不保存用户角色或权限。登录后和刷新页面时，通过 `/api/v1/auth/me` 获取数据库中的当前用户，资料只保存在 Zustand 内存状态中；校验失败会清除 token 和用户信息。

前端菜单和路由守卫只控制展示。API 是否允许读取或修改数据，由后端数据库角色校验决定；修改浏览器缓存或页面不能获得接口权限。

执行 `npm test` 可验证本地角色伪造、当前用户校验失败和退出清理；测试只替换 API 边界，使用实际 store 和 token 存储代码。

## 构建

```bash
npm run build
```

构建产物输出到 `dist/`，Dockerfile 会通过 Nginx 托管静态资源，并将 `/api/` 反向代理到 compose 中的 `api:8080`。

## 目录

```text
src/
  api/          接口请求封装
  components/   通用组件
  layouts/      应用布局
  pages/        登录、校友端、管理端、数据大屏、个人资料页面
  router/       路由和权限守卫
  store/        Zustand 状态
  types/        业务类型
  utils/        权限和字典工具
```
