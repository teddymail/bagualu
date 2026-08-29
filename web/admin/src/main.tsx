import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Alert, Avatar, Badge, Button, Card, Col, ConfigProvider, Descriptions as AntDescriptions, Drawer, Empty, Form, Input, Layout, Menu, Modal, Progress, Row, Select,
  Space, Statistic, Switch, Table, Tag, Typography, Upload, message, theme as antdTheme,
} from "antd";
import type { MenuProps, TableProps } from "antd";
import {
  ApiOutlined, AreaChartOutlined, DashboardOutlined, FileTextOutlined, KeyOutlined, LinkOutlined,
  NodeIndexOutlined, PlayCircleOutlined, SettingOutlined, TeamOutlined, ThunderboltOutlined, UploadOutlined,
} from "@ant-design/icons";
import "antd/dist/reset.css";
import "./style.css";
import { api, CoreInstallStatus, CoreStatus, Summary } from "./api";

const { Header, Sider, Content } = Layout;
type Page = "dashboard" | "runtime" | "upstreams" | "nodes" | "groups" | "tests" | "reports" | "logs" | "keys" | "subscriptions" | "settings";
type RowData = Record<string, unknown> & { id?: string; name?: string; status?: string; protocol?: string; address?: string; latency?: number; speed?: number };

const nav: MenuProps["items"] = [
  { type: "group", label: "资源", children: [
    { key: "dashboard", icon: <DashboardOutlined />, label: "仪表盘" },
    { key: "upstreams", icon: <LinkOutlined />, label: "上游订阅" },
    { key: "nodes", icon: <NodeIndexOutlined />, label: "节点" },
    { key: "groups", icon: <TeamOutlined />, label: "分组" },
    { key: "subscriptions", icon: <ApiOutlined />, label: "订阅链接" },
  ]},
  { type: "group", label: "测试", children: [
    { key: "runtime", icon: <ThunderboltOutlined />, label: "运行后台" },
    { key: "tests", icon: <PlayCircleOutlined />, label: "测试中心" },
    { key: "reports", icon: <AreaChartOutlined />, label: "流量报表" },
  ]},
  { type: "group", label: "系统", children: [
    { key: "settings", icon: <SettingOutlined />, label: "系统设置" },
    { key: "logs", icon: <FileTextOutlined />, label: "日志" },
    { key: "keys", icon: <KeyOutlined />, label: "API Key" },
  ]},
];

function useApi<T>(path: string | null, fallback: T) {
  const [data, setData] = useState<T>(fallback);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const reload = () => {
    setError("");
    if (!path) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api.get<T>(path).then(setData).catch((e: Error) => setError(e.message)).finally(() => setLoading(false));
  };
  useEffect(reload, [path]);
  return { data, loading, error, reload };
}

function EmptyData({ title = "暂无数据", hint = "连接 API 后，这里会显示实时数据。" }: { title?: string; hint?: string }) {
  return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<><div>{title}</div><Typography.Text type="secondary">{hint}</Typography.Text></>} />;
}

function Login({ onLogin }: { onLogin: () => void }) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const submit = async (values: { password: string }) => {
    setLoading(true);
    setError("");
    try {
      await api.auth.login("admin", values.password);
      onLogin();
    } catch (e) {
      setError(e instanceof Error ? e.message : "登录失败");
    } finally {
      setLoading(false);
    }
  };
  return <div className="login-shell">
    <Card className="login-panel">
      <Space direction="vertical" size="large" style={{ width: "100%" }}>
        <Space align="center" size="middle">
          <Avatar shape="square" size={48} icon={<ThunderboltOutlined />} />
          <div><Typography.Title level={2} style={{ margin: 0 }}>八卦炉</Typography.Title><Typography.Text type="secondary">链路控制台</Typography.Text></div>
        </Space>
        <Typography.Paragraph className="login-copy">管理节点、出口与测试任务。</Typography.Paragraph>
      <Form layout="vertical" onFinish={submit}>
        <Form.Item label="管理员密码" name="password" rules={[{ required: true, message: "请输入管理员密码" }]}>
          <Input.Password autoFocus />
        </Form.Item>
        {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />}
        <Button type="primary" htmlType="submit" loading={loading} block>进入控制台</Button>
      </Form>
      </Space>
    </Card>
  </div>;
}

function StateTag({ value }: { value?: string }) {
  const good = ["running", "healthy", "success", "available", "已启用"].includes(String(value).toLowerCase());
  return <Tag color={good ? "success" : value ? "warning" : "default"}>{value || "未知"}</Tag>;
}

function Dashboard({ go }: { go: (p: Page) => void }) {
  const summary = useApi<Summary>("/dashboard/summary", {});
  const core = useApi<CoreStatus>("/system/core/status", {});
  const values = summary.data;
  return <PageShell title="仪表盘" action={<Button onClick={summary.reload}>刷新</Button>}>
    {summary.error && <Alert type="info" showIcon message="仪表盘 API 尚未提供，显示空状态" description={summary.error} />}
    <Card size="small" className="evidence"><Space wrap><Typography.Text strong>八卦炉</Typography.Text><StateTag value="running" /><Typography.Text type="secondary">→</Typography.Text><Typography.Text strong>Mihomo</Typography.Text><StateTag value={core.data.running ? "running" : "未就绪"} /><Typography.Text type="secondary">→</Typography.Text><Typography.Text strong>测试链路</Typography.Text><StateTag value={values.running ? "running" : "idle"} /></Space></Card>
    <Row gutter={[12, 12]} className="stats">
      {[["节点总数", values.node_count || 0], ["可用节点", values.node_status_counts?.active || 0], ["异常节点", (values.node_status_counts?.unreachable || 0) + (values.node_status_counts?.expired || 0)], ["禁用节点", values.node_status_counts?.disabled || 0], ["分组", values.group_count || 0], ["测试队列", values.queue_length || 0]].map(([title, value]) => <Col xs={12} sm={8} lg={4} key={String(title)}><Card><Statistic title={title} value={value} /></Card></Col>)}
    </Row>
    <Row gutter={16}><Col xs={24} lg={14}><Card title="系统状态"><Descriptions items={[
      ["八卦炉", "运行中"], ["Mihomo", core.data.version ? `已连接 · ${core.data.version}` : "未就绪"], ["管理后台", "本地资源"], ["数据更新时间", "等待 API 返回"],
    ]} /></Card></Col><Col xs={24} lg={10}><Card title="下一步"><Space direction="vertical"><Typography.Text>从上游导入节点后即可开始测试。</Typography.Text><Space><Button type="primary" onClick={() => go("upstreams")}>管理上游</Button><Button onClick={() => go("runtime")}>运行后台</Button></Space></Space></Card></Col></Row>
  </PageShell>;
}

function Descriptions({ items }: { items: string[][] }) { return <AntDescriptions column={1} size="small" items={items.map(([label, children]) => ({ key: label, label, children }))} />; }

function Upstreams({ go }: { go: (p: Page) => void }) {
  const result = useApi<{ upstreams?: RowData[] }>("/upstreams", {});
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<RowData>();
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();
  const rows = result.data.upstreams || [];
  const submit = async (values: RowData) => {
    setSaving(true);
    try {
      if (editing?.id) await api.put(`/upstreams/${editing.id}`, values);
      else await api.post("/upstreams", values);
      message.success(editing ? "上游已更新" : "上游已创建");
      setOpen(false);
      result.reload();
    } catch (e) {
      message.error(e instanceof Error ? e.message : "保存失败");
    } finally {
      setSaving(false);
    }
  };
  const remove = (row: RowData) => {
    Modal.confirm({
      title: "删除上游订阅？",
      content: row.name,
      onOk: async () => {
        try {
          await api.remove(`/upstreams/${row.id}`);
          message.success("上游已删除");
          result.reload();
        } catch (e) {
          message.error(e instanceof Error ? e.message : "删除失败");
        }
      },
    });
  };
  const edit = (row?: RowData) => {
    setEditing(row);
    form.setFieldsValue(row || { format: "auto", refresh_interval_seconds: 3600, enabled: true });
    setOpen(true);
  };
  return <PageShell title="上游订阅" action={<Space><Button onClick={result.reload}>刷新</Button><Button type="primary" onClick={() => edit()}>新建上游</Button></Space>}>
    {result.error && <Alert type="info" showIcon message="接口暂不可用，保留空数据状态" description={result.error} />}
    <Card><Table rowKey="id" loading={result.loading} dataSource={rows} columns={[
      { title: "名称", dataIndex: "name" },
      { title: "订阅地址", dataIndex: "url", render: (v) => <span className="mono">{String(v || "—")}</span> },
      { title: "格式", dataIndex: "format" },
      { title: "状态", dataIndex: "enabled", render: (v) => <StateTag value={v ? "已启用" : "已停用"} /> },
      { title: "操作", render: (_, row) => <Space><Button size="small" onClick={() => edit(row)}>编辑</Button><Button size="small" onClick={async () => { try { await api.post(`/upstreams/${row.id}/refresh`); message.success("已提交刷新任务"); } catch (e) { message.error(e instanceof Error ? e.message : "刷新失败"); } }}>刷新订阅</Button><Button size="small" onClick={async () => { try { const result = await api.post<{ node_count: number; skipped_count?: number }>(`/upstreams/${row.id}/tests/throughput`); const skipped = Number(result.skipped_count || 0); message.success(`已提交 ${result.node_count} 个节点的测速任务${skipped ? `，${skipped} 个等待队列空闲后重试` : ""}`); go("tests"); } catch (e) { message.error(e instanceof Error ? e.message : "测速失败"); } }}>订阅测速</Button><Button size="small" danger onClick={() => remove(row)}>删除</Button></Space> },
    ]} locale={{ emptyText: <EmptyData title="暂无上游订阅" hint="创建上游后即可导入节点。" /> }} pagination={{ pageSize: 10 }} /></Card>
    <Modal title={editing ? "编辑上游订阅" : "新建上游订阅"} open={open} onCancel={() => setOpen(false)} footer={null}>
      <Form form={form} layout="vertical" onFinish={submit}>
        <Form.Item label="名称" name="name" rules={[{ required: true, message: "请输入名称" }]}><Input /></Form.Item>
        <Form.Item label="订阅地址" name="url" rules={[{ required: true, message: "请输入订阅地址" }]}><Input /></Form.Item>
        <Form.Item label="格式" name="format"><Select options={[{ value: "auto", label: "自动识别" }, { value: "clash", label: "Clash / Mihomo" }, { value: "base64", label: "Base64 URI" }]} /></Form.Item>
        <Form.Item label="刷新间隔（秒）" name="refresh_interval_seconds"><Input type="number" /></Form.Item>
        <Form.Item label="启用" name="enabled" valuePropName="checked"><Switch /></Form.Item>
        <Button type="primary" htmlType="submit" loading={saving} block>保存</Button>
      </Form>
    </Modal>
  </PageShell>;
}

function DataPage({ page, title, endpoint, columns, createLabel, form }: { page: Page; title: string; endpoint: string; columns: TableProps<RowData>["columns"]; createLabel?: string; form?: React.ReactNode }) {
  const result = useApi<RowData[] | { items?: RowData[] }>(endpoint, []);
  const rows = Array.isArray(result.data) ? result.data : result.data.items || Object.values(result.data).find(Array.isArray) as RowData[] || [];
  const [open, setOpen] = useState(false);
  const submit = async (values: RowData) => {
    try {
      await api.post(endpoint, values);
      message.success(`${createLabel}已创建`);
      setOpen(false);
      result.reload();
    } catch (e) {
      message.error(e instanceof Error ? e.message : "保存失败");
    }
  };
  return <PageShell title={title} action={<Space><Button onClick={result.reload}>刷新</Button>{createLabel && <Button type="primary" onClick={() => setOpen(true)}>新建{createLabel}</Button>}</Space>}>
    {result.error && <Alert type="info" showIcon message="接口暂不可用，保留空数据状态" description={result.error} />}
    <Card className="table-card"><Table rowKey="id" loading={result.loading} dataSource={rows} columns={columns} locale={{ emptyText: <EmptyData title={`暂无${title}`} hint="可通过上方操作创建第一条记录。" /> }} pagination={{ pageSize: 10 }} /></Card>
    <Modal title={`新建${createLabel}`} open={open} onCancel={() => setOpen(false)} footer={null}><Form layout="vertical" onFinish={submit}><Form.Item label="名称" name="name" rules={[{ required: true }]}><Input placeholder="输入名称" /></Form.Item>{form}<Button type="primary" htmlType="submit">保存</Button></Form></Modal>
  </PageShell>;
}

type SubscriptionPreview = {
  format?: string;
  compatible_count?: number;
  skipped_count?: number;
  skipped?: string[];
  regions?: string[];
  uris?: string[];
};

function Subscriptions() {
  const result = useApi<{ subscription_links?: RowData[] }>("/subscription-links", {});
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [form] = Form.useForm();
  const [token, setToken] = useState("");
  const [tokenOpen, setTokenOpen] = useState(false);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const rows = result.data.subscription_links || [];
  const formats = [
    { value: "clash", label: "Clash / Mihomo / OpenClash / Nikki" },
    { value: "base64", label: "Base64 URI / v2rayNG / PassWall / SSR+" },
    { value: "sing-box", label: "sing-box" },
    { value: "dae", label: "dae / daed" },
  ];
  const makeURL = (value: string, format?: string) => {
    const suffix = format ? `?format=${encodeURIComponent(format)}` : "";
    return `${window.location.origin}/api/v1/subscribe/${encodeURIComponent(value)}${suffix}`;
  };
  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      message.success("订阅链接已复制");
    } catch {
      message.error("复制失败，请手动复制");
    }
  };
  const submit = async (values: RowData) => {
    setCreating(true);
    try {
      const created = await api.post<{ token: string }>("/subscription-links", {
        ...values,
        allowed_formats: values.allowed_formats || formats.map((item) => item.value),
      });
      setToken(created.token);
      setOpen(false);
      setTokenOpen(true);
      form.resetFields();
      result.reload();
      message.success("订阅链接已创建");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "创建失败");
    } finally {
      setCreating(false);
    }
  };
  const loadPreview = async (row: RowData, format?: string) => {
    try {
      const value = format || String(row.default_format || "clash");
      const data = await api.get<SubscriptionPreview>(`/subscription-links/${row.id}/preview?format=${encodeURIComponent(value)}`);
      setPreview(data);
      setPreviewOpen(true);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "预览失败");
    }
  };
  return <PageShell title="订阅链接" action={<Space><Button onClick={result.reload}>刷新</Button><Button type="primary" onClick={() => { form.setFieldsValue({ default_format: "clash", allowed_formats: formats.map((item) => item.value), healthy_only: true, enabled: true }); setOpen(true); }}>新建订阅链接</Button></Space>}>
    <Alert showIcon type="info" message="自动适配已启用" description="未指定 format 时，八卦炉根据客户端 User-Agent 自动返回 Clash、Base64、sing-box 或 dae/daed 格式；未知客户端回退到默认格式。" />
    {result.error && <Alert type="warning" showIcon message="接口暂不可用" description={result.error} />}
    <Card className="table-card"><Table rowKey="id" loading={result.loading} dataSource={rows} columns={[
      { title: "名称", dataIndex: "name" },
      { title: "分组", dataIndex: "group_id", render: (value) => <span className="mono">{String(value || "—")}</span> },
      { title: "默认格式", dataIndex: "default_format", render: (value) => <Tag color="blue">{String(value || "clash")}</Tag> },
      { title: "允许格式", dataIndex: "allowed_formats", render: (value) => <Space wrap>{(Array.isArray(value) ? value : []).map((item) => <Tag key={String(item)}>{String(item)}</Tag>)}</Space> },
      { title: "兼容预览", render: (_, row) => <Button size="small" onClick={() => loadPreview(row)}>查看适配</Button> },
      { title: "状态", dataIndex: "enabled", render: (value) => <StateTag value={value ? "已启用" : "已停用"} /> },
      { title: "最近访问", dataIndex: "last_access_at", render: (value) => value ? String(value) : "未访问" },
    ]} locale={{ emptyText: <EmptyData title="暂无订阅链接" hint="创建后可直接复制自动适配地址给各种客户端导入。" /> }} pagination={{ pageSize: 10 }} scroll={{ x: 980 }} /></Card>
    <Modal title="新建订阅链接" open={open} onCancel={() => setOpen(false)} footer={null} destroyOnClose>
      <Form form={form} layout="vertical" onFinish={submit}>
        <Form.Item label="名称" name="name" rules={[{ required: true, message: "请输入名称" }]}><Input placeholder="例如：主力节点" /></Form.Item>
        <Form.Item label="资源分组 ID" name="group_id" rules={[{ required: true, message: "请输入分组 ID" }]}><Input placeholder="填写分组 ID" /></Form.Item>
        <Form.Item label="默认格式" name="default_format"><Select options={formats} /></Form.Item>
        <Form.Item label="允许格式" name="allowed_formats" extra="建议全部开启，客户端会按 User-Agent 自动选择。"><Select mode="multiple" options={formats} /></Form.Item>
        <Form.Item label="仅健康节点" name="healthy_only" valuePropName="checked"><Switch /></Form.Item>
        <Form.Item label="启用" name="enabled" valuePropName="checked"><Switch /></Form.Item>
        <Button type="primary" htmlType="submit" loading={creating} block>创建并生成订阅地址</Button>
      </Form>
    </Modal>
    <Modal title="订阅地址已生成" open={tokenOpen} onCancel={() => setTokenOpen(false)} footer={null} width={720}>
      <Alert type="warning" showIcon message="请立即保存这些地址" description="完整 Token 只在创建成功后显示一次；后续可通过轮换 Token 重新生成。" />
      <Space direction="vertical" style={{ width: "100%" }}>
        <Typography.Text strong>自动适配地址</Typography.Text>
        <Space.Compact style={{ width: "100%" }}><Input readOnly value={token ? makeURL(token) : ""} /><Button onClick={() => copy(makeURL(token))}>复制</Button></Space.Compact>
        {formats.map((item) => <div key={item.value}><Typography.Text type="secondary">{item.label}</Typography.Text><Space.Compact style={{ width: "100%" }}><Input readOnly value={token ? makeURL(token, item.value) : ""} /><Button onClick={() => copy(makeURL(token, item.value))}>复制</Button></Space.Compact></div>)}
      </Space>
    </Modal>
    <Modal title={`兼容性预览 · ${preview?.format || ""}`} open={previewOpen} onCancel={() => setPreviewOpen(false)} footer={null}>
      <Descriptions items={[["兼容节点", String(preview?.compatible_count || 0)], ["跳过节点", String(preview?.skipped_count || preview?.skipped?.length || 0)], ["地区组", (preview?.regions || []).join("、") || "无"]]} />
      {(preview?.uris?.length || 0) > 0 && <><Typography.Text strong>可导入 URI</Typography.Text><Input.TextArea value={preview?.uris?.join("\n")} readOnly rows={6} /></>}
      {(preview?.skipped?.length || 0) > 0 && <><Typography.Text strong>跳过原因</Typography.Text><Input.TextArea value={preview?.skipped?.join("\n")} readOnly rows={6} /></>}
    </Modal>
  </PageShell>;
}

function Nodes({ onHistory }: { onHistory: (node: RowData) => void }) {
  const result = useApi<{ nodes?: RowData[] }>("/nodes", {});
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<RowData | null>(null);
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [regionFilter, setRegionFilter] = useState("");
  const [form] = Form.useForm();
  const rows = result.data.nodes || [];
  const regions = Array.from(new Set(rows.map((row) => String(row.region || "")).filter(Boolean))).sort();
  const visibleRows = rows.filter((row) => {
    const text = `${String(row.name || "")} ${String(row.address || "")} ${String(row.endpoint_ip || "")} ${String(row.protocol || "")}`.toLowerCase();
    return (!search || text.includes(search.toLowerCase())) && (!statusFilter || row.status === statusFilter) && (!regionFilter || row.region === regionFilter);
  });
  const edit = (node?: RowData) => {
    setEditing(node || null);
    form.resetFields();
    if (node) form.setFieldsValue({ name: node.name, region: node.region });
    setOpen(true);
  };
  const submit = async (values: RowData) => {
    setSaving(true);
    try {
      if (editing?.id) {
        await api.put(`/nodes/${encodeURIComponent(editing.id)}`, values);
        message.success("手动节点已更新");
      } else {
        await api.post("/nodes", values);
        message.success("手动节点已创建");
      }
      setOpen(false);
      form.resetFields();
      result.reload();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存节点失败");
    } finally {
      setSaving(false);
    }
  };
  const remove = (node: RowData) => {
    Modal.confirm({
      title: "删除手动节点？",
      content: node.name || node.address,
      onOk: async () => {
        try {
          await api.remove(`/nodes/${encodeURIComponent(String(node.id))}`);
          message.success("节点已删除");
          result.reload();
        } catch (error) {
          message.error(error instanceof Error ? error.message : "删除节点失败");
        }
      },
    });
  };
  const runTest = async (node: RowData, kind: "connectivity" | "ping" | "throughput") => {
    try {
      await api.post(`/nodes/${encodeURIComponent(String(node.id))}/tests/${kind}`);
      message.success(`${measurementKind(kind)}任务已提交`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "提交测试失败");
    }
  };
  const columns: TableProps<RowData>["columns"] = [
    { title: "名称", dataIndex: "name", sorter: (a, b) => String(a.name || "").localeCompare(String(b.name || ""), "zh-CN"), render: (value) => value || "—" },
    { title: "来源", dataIndex: "source_type", render: (value) => <Tag color={value === "manual" ? "blue" : "geekblue"}>{value === "manual" ? "手动管理" : "订阅同步"}</Tag> },
    { title: "协议", dataIndex: "protocol", filters: ["vless", "vmess", "trojan", "ss", "socks5"].map((value) => ({ text: value.toUpperCase(), value })), onFilter: (value, row) => String(row.protocol || "").toLowerCase() === String(value), render: (value) => value ? <Tag>{String(value).toUpperCase()}</Tag> : "—" },
    { title: "地址", dataIndex: "address", render: (value) => <span className="mono">{String(value || "—")}</span> },
    { title: "状态", dataIndex: "status", filters: ["active", "unreachable", "disabled", "expired", "invalid"].map((value) => ({ text: value, value })), onFilter: (value, row) => String(row.status || "") === String(value), render: (value) => <StateTag value={value as string} /> },
    { title: "延迟", dataIndex: "latency_ms", sorter: (a, b) => numericValue(a.latency_ms) - numericValue(b.latency_ms), render: (value) => formatLatency(value) },
    { title: "吞吐", dataIndex: "speed_bytes_per_sec", sorter: (a, b) => numericValue(a.speed_bytes_per_sec) - numericValue(b.speed_bytes_per_sec), render: (value) => value ? formatRate(Number(value)) : "—" },
    { title: "操作", key: "actions", fixed: "right", width: 360, render: (_, row) => <Space size={4} wrap><Button size="small" onClick={() => onHistory(row)}>测速历史</Button><Button size="small" onClick={() => runTest(row, "connectivity")}>连通性</Button><Button size="small" onClick={() => runTest(row, "ping")}>Ping</Button><Button size="small" type="primary" ghost onClick={() => runTest(row, "throughput")}>下载测速</Button>{row.source_type === "manual" ? <><Button size="small" onClick={() => edit(row)}>编辑</Button><Button size="small" danger onClick={() => remove(row)}>删除</Button></> : null}</Space> },
  ];
  return <PageShell title="节点" action={<Space><Button onClick={result.reload}>刷新</Button><Button type="primary" onClick={() => edit()}>一键导入节点字符串</Button></Space>}>
    {result.error && <Alert type="error" showIcon message="节点加载失败" description={result.error} />}
    <Space wrap className="filter-bar"><Input.Search allowClear placeholder="搜索名称、地址、入口 IP 或协议" onSearch={setSearch} onChange={(event) => setSearch(event.target.value)} style={{ width: 320 }} /><Select allowClear placeholder="状态" style={{ width: 160 }} onChange={(value) => setStatusFilter(value || "")} options={["active", "unreachable", "endpoint_ip_unreachable", "disabled", "expired", "invalid"].map((value) => ({ value, label: value }))} /><Select allowClear placeholder="地区" style={{ width: 140 }} onChange={(value) => setRegionFilter(value || "")} options={regions.map((value) => ({ value, label: value }))} /><Typography.Text type="secondary">显示 {visibleRows.length} / {rows.length}</Typography.Text></Space>
    <Card className="table-card"><Table rowKey="id" loading={result.loading} dataSource={visibleRows} columns={columns} locale={{ emptyText: <EmptyData title="暂无节点" hint="可添加手动节点，或先刷新上游订阅。" /> }} pagination={{ pageSize: 10 }} scroll={{ x: 1420 }} /></Card>
    <Modal title={editing ? "编辑手动节点" : "一键导入手动节点"} open={open} onCancel={() => setOpen(false)} footer={null} destroyOnClose>
      <Form form={form} layout="vertical" onFinish={submit} initialValues={{}}>
        <Form.Item label="节点 URI" name="uri" rules={editing ? undefined : [{ required: true, message: "请输入单个节点 URI" }]} extra={editing ? "留空则只更新名称或地域；填写后会替换连接配置。" : "支持 vless、vmess、trojan、ss、socks5 等单节点 URI。"}><Input.TextArea rows={4} placeholder="vless://、vmess://、trojan://、ss://" /></Form.Item>
        <Form.Item label="名称" name="name" extra="留空时使用 URI 中的节点名称。"><Input placeholder="例如：家庭宽带" /></Form.Item>
        <Form.Item label="地域" name="region"><Input placeholder="例如：US / JP" /></Form.Item>
        <Button type="primary" htmlType="submit" loading={saving}>{editing ? "保存修改" : "导入并保存"}</Button>
      </Form>
    </Modal>
  </PageShell>;
}

function Tests() {
  const jobs = useApi<RowData[] | { jobs?: RowData[]; items?: RowData[] }>("/jobs", []);
  const nodes = useApi<{ nodes?: RowData[] }>("/nodes?status=active", {});
  const policy = useApi<RowData>("/test-policy", {});
  const rows = Array.isArray(jobs.data) ? jobs.data : jobs.data.jobs || jobs.data.items || [];
  const nodeRows = nodes.data.nodes || [];
  const [clearing, setClearing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [nodeID, setNodeID] = useState("");
  const [testKind, setTestKind] = useState("throughput");
  const clearActiveTasks = async () => {
    setClearing(true);
    try {
      await api.post("/jobs/clear?scope=all&purge=true");
      message.success("任务数据已清空");
      jobs.reload();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "清空任务失败");
    } finally {
      setClearing(false);
    }
  };
  useEffect(() => {
    const timer = window.setInterval(jobs.reload, 3000);
    return () => window.clearInterval(timer);
  }, []);
  const submitTest = async () => {
    if (!nodeID) {
      message.warning("请选择一个活动节点");
      return;
    }
    setSubmitting(true);
    try {
      await api.post(`/nodes/${encodeURIComponent(nodeID)}/tests/${testKind}`);
      message.success("测试任务已提交");
      jobs.reload();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "提交测试失败");
    } finally {
      setSubmitting(false);
    }
  };
  const windows = Array.isArray(policy.data.allowed_windows) ? policy.data.allowed_windows.join("、") : String(policy.data.allowed_windows || "全天");
  return <PageShell title="测试中心" action={<Space><Button danger loading={clearing} onClick={clearActiveTasks}>清空任务数据</Button><Button onClick={() => { jobs.reload(); nodes.reload(); policy.reload(); }}>刷新</Button></Space>}><Row gutter={16}><Col xs={24} lg={8}><Card title="发起测试"><Space direction="vertical" style={{ width: "100%" }}><Select showSearch value={nodeID || undefined} loading={nodes.loading} placeholder="选择活动节点" style={{ width: "100%" }} optionFilterProp="label" onChange={setNodeID} options={nodeRows.map((node) => ({ value: String(node.id), label: `${String(node.name || node.address)} · ${String(node.protocol || "").toUpperCase()}` }))} /><Select value={testKind} style={{ width: "100%" }} onChange={setTestKind} options={[{ value: "connectivity", label: "连通性测试" }, { value: "ping", label: "Ping 测试" }, { value: "throughput", label: "下载测速" }]} /><Button type="primary" block loading={submitting} disabled={!nodeID} onClick={submitTest}>提交测试任务</Button></Space><Descriptions items={[["执行方式", "固定单线程"], ["节点范围", "手动节点和订阅节点"], ["每日测速", policy.data.throughput_enabled === false ? "已关闭" : "已启用"], ["测速时段", windows], ["测速源", String(policy.data.throughput_url || "未配置")], ["每节点流量", formatMeasurementBytes(policy.data.throughput_bytes)]]} /></Card></Col><Col xs={24} lg={16}><Card title="正在处理的任务"><Table rowKey="id" loading={jobs.loading} dataSource={rows} columns={[{ title: "任务", dataIndex: "id", render: (v) => <span className="mono">{String(v || "—").slice(0, 8)}</span> }, { title: "对象", dataIndex: "entity_id", render: (v) => <span className="mono">{String(v || "—").slice(0, 8)}</span> }, { title: "类型", dataIndex: "kind" }, { title: "状态", dataIndex: "status", render: (v) => <StateTag value={v} /> }, { title: "进度", dataIndex: "progress", render: (v) => <Progress percent={Number(v) || 0} size="small" /> }]} locale={{ emptyText: <EmptyData title="暂无正在处理的任务" /> }} pagination={{ pageSize: 10 }} /></Card></Col></Row></PageShell>;
}

function formatRate(bytesPerSecond: number) {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond < 0) return "0 B/s";
  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let value = bytesPerSecond;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${Math.round(value)} ${units[unit]}`;
}

function formatLatency(value: unknown) {
  const latency = Number(value);
  return Number.isFinite(latency) && latency > 0 ? `${Math.round(latency)} ms` : "—";
}

function formatScore(value: unknown) {
  const score = Number(value);
  return Number.isFinite(score) ? String(Math.round(score)) : "—";
}

function formatScoreMetric(value: unknown, samples: unknown) {
  return Number(samples) > 0 ? formatScore(value) : "未测";
}

function numericValue(value: unknown) {
  const number = Number(value);
  return Number.isFinite(number) ? number : -1;
}

type RuntimeTraffic = {
  connections?: number;
  bagualu_download_bytes?: number;
  bagualu_upload_bytes?: number;
  wan_download_bytes?: number;
  wan_upload_bytes?: number;
  interface?: string;
  error?: string;
};

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const decimals = unit === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(decimals)} ${units[unit]}`;
}

function Runtime() {
  const traffic = useApi<RuntimeTraffic>("/runtime/traffic", {});
  const runtime = useApi<{ tasks?: RowData[]; core?: CoreStatus; running?: boolean; queue_length?: number }>("/runtime/summary", {});
  const [downloadSamples, setDownloadSamples] = useState<number[]>([]);
  const [uploadSamples, setUploadSamples] = useState<number[]>([]);
  const [lastBytes, setLastBytes] = useState<{ download?: number; upload?: number }>({});
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  useEffect(() => {
    const timer = window.setInterval(() => {
      traffic.reload();
      runtime.reload();
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);
  useEffect(() => {
    const downloadBytes = Number(traffic.data.bagualu_download_bytes || 0);
    const uploadBytes = Number(traffic.data.bagualu_upload_bytes || 0);
    const downloadSpeed = lastBytes.download === undefined ? 0 : Math.max(0, downloadBytes - lastBytes.download);
    const uploadSpeed = lastBytes.upload === undefined ? 0 : Math.max(0, uploadBytes - lastBytes.upload);
    setDownloadSamples((current) => [...current, downloadSpeed].slice(-30));
    setUploadSamples((current) => [...current, uploadSpeed].slice(-30));
    setLastBytes({ download: downloadBytes, upload: uploadBytes });
  }, [traffic.data.bagualu_download_bytes, traffic.data.bagualu_upload_bytes]);
  const sampleCount = Math.max(downloadSamples.length, uploadSamples.length);
  const max = Math.max(...downloadSamples, ...uploadSamples, 1);
  const chart = { left: 66, right: 636, top: 22, bottom: 224 };
  const makePoints = (samples: number[]) => samples.map((value, index) => {
    const x = chart.left + (index / Math.max(sampleCount - 1, 1)) * (chart.right - chart.left);
    const y = chart.bottom - (value / max) * (chart.bottom - chart.top);
    return `${x},${y}`;
  }).join(" ");
  const downloadPoints = makePoints(downloadSamples);
  const uploadPoints = makePoints(uploadSamples);
  const downloadAreaPoints = `${chart.left},${chart.bottom} ${downloadPoints} ${chart.right},${chart.bottom}`;
  const currentDownload = downloadSamples[downloadSamples.length - 1] || 0;
  const currentUpload = uploadSamples[uploadSamples.length - 1] || 0;
  const peakRate = Math.max(...downloadSamples, ...uploadSamples, 0);
  const axisLabels = [1, 0.75, 0.5, 0.25, 0].map((ratio) => ({
    ratio,
    label: formatRate(max * ratio),
  }));
  const hoverDownload = hoverIndex === null ? 0 : downloadSamples[hoverIndex] || 0;
  const hoverUpload = hoverIndex === null ? 0 : uploadSamples[hoverIndex] || 0;
  const hoverRate = Math.max(hoverDownload, hoverUpload);
  const hoverX = hoverIndex === null ? 0 : chart.left + (hoverIndex / Math.max(sampleCount - 1, 1)) * (chart.right - chart.left);
  const hoverY = chart.bottom - (hoverRate / max) * (chart.bottom - chart.top);
  const tooltipX = Math.min(Math.max(hoverX - 72, chart.left), chart.right - 144);
  const bagualuDownloadTotal = Number(traffic.data.bagualu_download_bytes || 0);
  const bagualuUploadTotal = Number(traffic.data.bagualu_upload_bytes || 0);
  const wanDownloadTotal = Number(traffic.data.wan_download_bytes || 0);
  const wanUploadTotal = Number(traffic.data.wan_upload_bytes || 0);
  return <PageShell title="运行后台" action={<Button onClick={() => { traffic.reload(); runtime.reload(); }}>刷新</Button>}>
    <Row gutter={16}><Col xs={24} lg={16}><Card title="八卦炉实时吞吐（受管 Mihomo）" extra={<Typography.Text type="secondary">每秒采样</Typography.Text>}>
      <div className="traffic-chart">
        <svg viewBox="0 0 680 260" role="img" aria-label={`最近 30 秒网卡下载速度，当前 ${formatRate(currentDownload)}`} onMouseMove={(event) => {
          if (sampleCount === 0) return;
          const bounds = event.currentTarget.getBoundingClientRect();
          const plotLeft = bounds.left + (chart.left / 680) * bounds.width;
          const plotWidth = ((chart.right - chart.left) / 680) * bounds.width;
          const ratio = Math.min(Math.max((event.clientX - plotLeft) / plotWidth, 0), 1);
          setHoverIndex(Math.round(ratio * (sampleCount - 1)));
        }} onMouseLeave={() => setHoverIndex(null)}>
          <defs>
            <linearGradient id="traffic-fill" x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor="#1677ff" stopOpacity="0.22" />
              <stop offset="100%" stopColor="#1677ff" stopOpacity="0.02" />
            </linearGradient>
          </defs>
          {axisLabels.map(({ ratio, label }) => {
            const y = chart.bottom - ratio * (chart.bottom - chart.top);
            return <g key={ratio}><line x1={chart.left} x2={chart.right} y1={y} y2={y} className="traffic-gridline" /><text x={chart.left - 10} y={y + 4} textAnchor="end" className="traffic-axis-label">{label}</text></g>;
          })}
          <polygon points={downloadAreaPoints} fill="url(#traffic-fill)" />
          <polyline points={downloadPoints} fill="none" stroke="#1677ff" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          <polyline points={uploadPoints} fill="none" stroke="#13a67a" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          {sampleCount > 0 && <><circle cx={chart.right} cy={chart.bottom - (currentDownload / max) * (chart.bottom - chart.top)} r="4" fill="#1677ff" /><circle cx={chart.right} cy={chart.bottom - (currentUpload / max) * (chart.bottom - chart.top)} r="4" fill="#13a67a" /></>}
          {hoverIndex !== null && <g className="traffic-hover">
            <line x1={hoverX} x2={hoverX} y1={chart.top} y2={chart.bottom} />
            <circle cx={hoverX} cy={hoverY} r="5" />
            <rect x={tooltipX} y={Math.max(4, hoverY - 48)} width="144" height="38" rx="6" />
            <text x={tooltipX + 12} y={Math.max(4, hoverY - 48) + 16}>下行 {formatRate(hoverDownload)}</text>
            <text x={tooltipX + 12} y={Math.max(4, hoverY - 48) + 30} className="traffic-tooltip-sub">上行 {formatRate(hoverUpload)} · 点 {hoverIndex + 1} / {sampleCount}</text>
          </g>}
        </svg>
        <div className="traffic-chart-caption"><span><i className="traffic-legend traffic-legend-download" />八卦炉下行 <i className="traffic-legend traffic-legend-upload" />八卦炉上行</span><span>最近 30 秒</span></div>
      </div>
      <div className="traffic-metrics">
        <Statistic title="八卦炉当前下载" value={formatRate(currentDownload)} />
        <Statistic title="八卦炉当前上传" value={formatRate(currentUpload)} />
        <Statistic title="八卦炉累计下载" value={formatBytes(bagualuDownloadTotal)} />
        <Statistic title="八卦炉累计上传" value={formatBytes(bagualuUploadTotal)} />
        <Statistic title="活动连接" value={traffic.data.connections || 0} />
      </div>
    </Card></Col><Col xs={24} lg={8}><Card title="流量口径"><Descriptions items={[["八卦炉流量", "受管 Mihomo 全部代理流量"], ["累计下行", formatBytes(bagualuDownloadTotal)], ["累计上行", formatBytes(bagualuUploadTotal)], ["WAN 网卡", String(traffic.data.interface || "未知")], ["WAN 累计下行", formatBytes(wanDownloadTotal)], ["WAN 累计上行", formatBytes(wanUploadTotal)]]} /></Card><Card title="服务状态"><Descriptions items={[["八卦炉", runtime.data.running ? "运行中" : "空闲"], ["Mihomo", runtime.data.core?.running ? `受管运行 · ${runtime.data.core.version || ""}` : "未就绪"], ["队列", String(runtime.data.queue_length || 0)], ["活动任务", String(runtime.data.tasks?.length || 0)], ["采样点", `${sampleCount} / 30`]]} /></Card><Card title="当前任务" className="section-card"><Table size="small" rowKey="id" pagination={false} dataSource={runtime.data.tasks || []} columns={[{ title: "类型", dataIndex: "kind" }, { title: "节点", dataIndex: "entity_id" }, { title: "状态", dataIndex: "status", render: (value) => <StateTag value={String(value || "")} /> }, { title: "进度", dataIndex: "progress", render: (value) => <Progress percent={Number(value) || 0} size="small" /> }]} locale={{ emptyText: <EmptyData title="暂无活动任务" /> }} /></Card></Col></Row>
  </PageShell>;
}

function Logs() {
  const result = useApi<{ logs?: RowData[] }>("/system/logs", {});
  useEffect(() => {
    const timer = window.setInterval(result.reload, 3000);
    return () => window.clearInterval(timer);
  }, []);
  return <PageShell title="日志" action={<Button onClick={result.reload}>刷新</Button>}>
    <Card><Table rowKey={(row) => String(row.job_id || row.time)} loading={result.loading} dataSource={result.data.logs || []} columns={[{ title: "时间", dataIndex: "time" }, { title: "级别", dataIndex: "level", render: (v) => <StateTag value={v} /> }, { title: "消息", dataIndex: "message" }, { title: "任务", dataIndex: "job_id", render: (v) => <span className="mono">{String(v || "—").slice(0, 8)}</span> }]} locale={{ emptyText: <EmptyData title="暂无日志" hint="任务执行后会在这里显示状态记录。" /> }} pagination={{ pageSize: 15 }} /></Card>
  </PageShell>;
}

function Settings() {
  const result = useApi<{ service?: string; core?: Record<string, unknown>; core_install?: CoreInstallStatus; go?: string; os?: string }>("/system/status", {});
  const core = result.data.core || {};
  const install = result.data.core_install || {};
  const [installing, setInstalling] = useState(false);
  const installCore = async () => {
    setInstalling(true);
    try {
      const response = await api.post<{ result?: { version?: string; asset?: string; verified?: boolean } }>("/system/core/install");
      message.success(`Mihomo ${response.result?.version || "已安装"} 已安装并重新启动`);
      result.reload();
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Mihomo 安装失败");
    } finally {
      setInstalling(false);
    }
  };
  const uploadCore = async (file: File) => {
    setInstalling(true);
    try {
      const response = await api.upload<{ result?: { version?: string } }>("/system/core/install/upload", file);
      message.success(`Mihomo ${response.result?.version || "文件"} 已安装并重新启动`);
      result.reload();
    } catch (e) {
      message.error(e instanceof Error ? e.message : "Mihomo 文件安装失败");
    } finally {
      setInstalling(false);
    }
  };
  return <PageShell title="系统设置" action={<Space><Button onClick={result.reload}>刷新状态</Button><Button type="primary" href="/cgi-bin/luci/admin/services/bagualu/config" target="_blank">在 LuCI 中配置</Button></Space>}>
    <Alert type="info" showIcon message="系统参数由 OpenWrt LuCI 统一管理" description="管理后台只展示运行状态。启用、端口、数据目录、Mihomo、WAN 带宽和每日测速策略请在 LuCI 中修改，避免出现两套配置。" />
    {!install.installed && <Alert type="warning" showIcon message="Mihomo 内核未安装，测试功能暂不可用" description={<Space direction="vertical"><Typography.Text>八卦炉会从 LuCI 中配置的发行版仓库下载匹配当前 OpenWrt 架构的官方内核，校验后安装到配置的路径并自动重启。若设备访问 GitHub 受限，可在电脑下载官方 Linux 文件后上传。</Typography.Text><Space wrap><Button type="primary" loading={installing} onClick={installCore}>下载并安装 Mihomo</Button><Upload accept=".gz,application/octet-stream" maxCount={1} showUploadList={false} beforeUpload={(file) => { void uploadCore(file); return false; }}><Button icon={<UploadOutlined />} loading={installing}>上传 Mihomo 文件</Button></Upload></Space></Space>} className="settings-grid" />}
    {install.installed && <Alert type="success" showIcon message={`Mihomo 已安装 · ${install.version || "版本由运行时返回"}`} description={`路径：${install.path || "—"} · 架构：${install.architecture || "—"}`} className="settings-grid" action={<Button loading={installing} onClick={installCore}>检查更新并重装</Button>} />}
    <Row gutter={16} className="settings-grid"><Col xs={24} lg={12}><Card title="八卦炉服务"><AntDescriptions column={1} items={[{ key: "service", label: "实际状态", children: <StateTag value={String(result.data.service || "未知")} /> }, { key: "platform", label: "运行平台", children: `${String(result.data.os || "—")} · ${String(result.data.go || "—")}` }, { key: "management", label: "管理入口", children: window.location.origin }]} /></Card></Col><Col xs={24} lg={12}><Card title="受管 Mihomo"><AntDescriptions column={1} items={[{ key: "state", label: "内核状态", children: <StateTag value={String(core.state || (core.available ? "running" : "stopped"))} /> }, { key: "pid", label: "PID", children: String(core.pid || "—") }, { key: "version", label: "版本", children: String(core.version || install.version || "—") }, { key: "control", label: "控制端口", children: String(core.control || "—") }, { key: "proxy", label: "代理端口", children: String(core.proxy || "—") }, { key: "restart", label: "自动拉起次数", children: String(core.auto_restarts || 0) }, { key: "error", label: "最近错误码", children: String(core.error_code || install.error || "—") }, { key: "source", label: "安装来源", children: String(install.source || "—") }]} /></Card></Col></Row>
  </PageShell>;
}

type MeasurementRow = {
  id: string;
  kind: string;
  success: boolean;
  latency_ms?: number;
  speed_bytes_per_sec?: number;
  bytes?: number;
  upload_bytes?: number;
  proxy_protocol?: string;
  test_url?: string;
  exit_ip?: string;
  baseline_target?: string;
  speed_source?: string;
  load_status?: string;
  background_upload_bps?: number;
  background_download_bps?: number;
  wan_download_before?: number;
  wan_download_after?: number;
  wan_upload_before?: number;
  wan_upload_after?: number;
  wan_download_capacity_bps?: number;
  wan_upload_capacity_bps?: number;
  load_threshold?: number;
  load_sample_duration_ms?: number;
  effective_download_duration_ms?: number;
  core_evidence?: { pid?: string; version?: string; node_name?: string; connection_id?: string; traffic_before?: number; traffic_after?: number; logical_connections?: number };
  error_code?: string;
  failure_stage?: string;
  infrastructure?: boolean;
  created_at: string;
};

function formatMeasurementBytes(value: unknown) {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes <= 0) return "—";
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024).toFixed(1)} KB`;
}

function formatMeasurementTime(value: unknown) {
  const date = new Date(String(value || ""));
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN");
}

function measurementKind(value: string) {
  return ({ connectivity: "连通性", ping: "Ping", throughput: "下载测速" } as Record<string, string>)[value] || value || "未知";
}

function NodeHistoryDrawer({ node, onClose }: { node: RowData | null; onClose: () => void }) {
  const result = useApi<{ measurements?: MeasurementRow[] }>(node?.id ? `/nodes/${encodeURIComponent(node.id)}/measurements?limit=50` : null, {});
  const measurements = result.data.measurements || [];
  return <Drawer title={`${String(node?.name || "节点")} · 测速历史`} open={Boolean(node)} onClose={onClose} width={680} extra={<Button onClick={result.reload} loading={result.loading}>刷新</Button>}>
    {result.error && <Alert type="error" showIcon message="测速历史加载失败" description={result.error} />}
    <AntDescriptions column={1} size="small" items={[{ key: "node", label: "节点", children: String(node?.name || "—") }, { key: "address", label: "地址", children: `${String(node?.address || "—")}:${String(node?.port || "—")}` }, { key: "count", label: "记录数", children: String(measurements.length) }]} />
    <Table<MeasurementRow> rowKey="id" size="small" loading={result.loading} dataSource={measurements} pagination={{ pageSize: 10 }} scroll={{ x: 1280 }} expandable={{ expandedRowRender: (row) => <AntDescriptions size="small" column={2} items={[{ key: "url", label: "测试地址", children: row.test_url || "—" }, { key: "source", label: "测速源", children: row.speed_source || "—" }, { key: "baseline", label: "外网基线", children: row.baseline_target || "—" }, { key: "load", label: "负载状态", children: row.load_status || "—" }, { key: "background-down", label: "WAN 背景下行", children: row.background_download_bps ? formatRate(row.background_download_bps) : "—" }, { key: "background-up", label: "WAN 背景上行", children: row.background_upload_bps ? formatRate(row.background_upload_bps) : "—" }, { key: "capacity", label: "配置带宽", children: row.wan_download_capacity_bps ? `下行 ${formatRate(row.wan_download_capacity_bps)} · 上行 ${formatRate(row.wan_upload_capacity_bps || 0)}` : "容量未知" }, { key: "sample", label: "WAN 采样", children: row.load_sample_duration_ms ? `${Math.round(row.load_sample_duration_ms)} ms · 下行 ${formatMeasurementBytes(Number(row.wan_download_after || 0) - Number(row.wan_download_before || 0))} · 上行 ${formatMeasurementBytes(Number(row.wan_upload_after || 0) - Number(row.wan_upload_before || 0))}` : "—" }, { key: "duration", label: "有效下载耗时", children: row.effective_download_duration_ms ? `${Math.round(row.effective_download_duration_ms)} ms` : "—" }, { key: "exit", label: "出口 IP", children: row.exit_ip || "—" }, { key: "core", label: "Mihomo 证据", children: row.core_evidence?.node_name ? `${row.core_evidence.node_name} · PID ${row.core_evidence.pid || "—"} · 连接 ${row.core_evidence.connection_id || "—"}` : "—" }]} /> }} locale={{ emptyText: <EmptyData title="暂无测速历史" hint="完成连通性、Ping 或下载测速后会在这里保留记录。" /> }} columns={[{ title: "时间", dataIndex: "created_at", width: 168, render: formatMeasurementTime }, { title: "类型", dataIndex: "kind", width: 96, render: measurementKind }, { title: "结果", dataIndex: "success", width: 88, render: (value, row) => <Tag color={row.infrastructure ? "warning" : value ? "success" : "error"}>{row.infrastructure ? "基础设施" : value ? "成功" : "失败"}</Tag> }, { title: "协议", dataIndex: "proxy_protocol", width: 88, render: (value) => value ? String(value).toUpperCase() : "—" }, { title: "延迟", dataIndex: "latency_ms", width: 88, render: formatLatency }, { title: "速度", dataIndex: "speed_bytes_per_sec", width: 112, render: (value) => Number(value) > 0 ? formatRate(Number(value)) : "—" }, { title: "下行数据", dataIndex: "bytes", width: 104, render: formatMeasurementBytes }, { title: "负载", dataIndex: "load_status", width: 120, render: (value) => value || "—" }, { title: "失败原因", width: 180, render: (_, row) => row.success ? "—" : <Typography.Text type="secondary">{row.error_code || row.failure_stage || "未知原因"}</Typography.Text> }]} />
  </Drawer>;
}

function PageShell({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) { return <div className="page"><div className="page-heading"><div><Typography.Title level={3}>{title}</Typography.Title><Typography.Text type="secondary">实时链路观测 · 运行数据持续刷新</Typography.Text></div>{action}</div>{children}</div>; }

function App() {
  const [authenticated, setAuthenticated] = useState(() => Boolean(sessionStorage.getItem("bagualu_admin_token")));
  const [page, setPage] = useState<Page>("dashboard");
  const [selectedNode, setSelectedNode] = useState<RowData | null>(null);
  useEffect(() => {
    const expire = () => setAuthenticated(false);
    window.addEventListener("bagualu-auth-expired", expire);
    return () => window.removeEventListener("bagualu-auth-expired", expire);
  }, []);
  const columns: TableProps<RowData>["columns"] = [{ title: "名称", dataIndex: "name", render: (v) => v || "—" }, { title: "协议", dataIndex: "protocol", render: (v) => v ? <Tag>{String(v).toUpperCase()}</Tag> : "—" }, { title: "地址", dataIndex: "address", render: (v) => <span className="mono">{String(v || "—")}</span> }, { title: "状态", dataIndex: "status", render: (v) => <StateTag value={v as string} /> }, { title: "延迟", dataIndex: "latency_ms", render: formatLatency }, { title: "吞吐", dataIndex: "speed_bytes_per_sec", render: (v) => Number(v) > 0 ? formatRate(Number(v)) : "—" }, { title: "评分", render: (_, row) => { const score = row.score as Record<string, unknown> | undefined; return score ? <Space size={2} wrap><Tag color="blue">总 {score.status === "unrated" ? "未评级" : formatScore(score.overall)}</Tag><Tag>延迟 {formatScoreMetric(score.latency, score.latency_samples)}</Tag><Tag>速度 {formatScoreMetric(score.speed, score.speed_samples)}</Tag><Tag>可用率 {formatScoreMetric(score.availability, score.availability_samples)}</Tag><StateTag value={String(score.status || "")} /></Space> : <Tag>未评级</Tag>; } }];
  const content = (() => {
    if (page === "dashboard") return <Dashboard go={setPage} />;
    if (page === "tests") return <Tests />;
    if (page === "runtime") return <Runtime />;
    if (page === "logs") return <Logs />;
    if (page === "upstreams") return <Upstreams go={setPage} />;
    if (page === "nodes") return <Nodes onHistory={setSelectedNode} />;
    if (page === "subscriptions") return <Subscriptions />;
    if (page === "settings") return <Settings />;
    const config: Record<string, [string, string, string | undefined]> = {
      groups: ["分组", "/groups", "分组"], reports: ["流量报表", "/reports/summary", undefined], keys: ["API Key", "/api-keys", "API Key"],
    };
    const [title, endpoint, create] = config[page];
    return <DataPage page={page} title={title} endpoint={endpoint} columns={columns} createLabel={create} form={page === "groups" ? <Form.Item label="描述" name="description"><Input.TextArea /></Form.Item> : page === "keys" ? <Form.Item label="分组 ID" name="group_id" rules={[{ required: true }]}><Input /></Form.Item> : undefined} />;
  })();
  if (!authenticated) return <Login onLogin={() => setAuthenticated(true)} />;
  return <ConfigProvider theme={{ algorithm: antdTheme.defaultAlgorithm }}>
    <Layout className="app-shell"><Sider className="app-sider" theme="light" breakpoint="lg" collapsedWidth={0}><div className="brand"><Avatar shape="square" size={40} icon={<ThunderboltOutlined />} /><div><Typography.Text strong>八卦炉</Typography.Text><Typography.Text type="secondary">链路控制台</Typography.Text></div></div><Menu theme="light" mode="inline" selectedKeys={[page]} items={nav} onClick={({ key }) => setPage(key as Page)} /></Sider><Layout><Header className="topbar"><Typography.Text type="secondary">{window.location.hostname || "localhost"}</Typography.Text><Space><Badge status="processing" text="八卦炉服务" /><Tag color="blue">MIHOMO 受管</Tag><Button type="link" onClick={() => setPage("settings")}>系统设置</Button><Button type="link" onClick={() => setPage("runtime")}>运行后台</Button></Space></Header><Content className="content">{content}</Content></Layout></Layout>
    <NodeHistoryDrawer node={selectedNode} onClose={() => setSelectedNode(null)} />
  </ConfigProvider>;
}

createRoot(document.getElementById("root")!).render(<React.StrictMode><App /></React.StrictMode>);
