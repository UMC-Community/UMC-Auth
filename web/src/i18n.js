import { ref } from 'vue'

const STORAGE_KEY = 'umc_auth_locale'
const supported = ['en', 'zh']

const zh = {
  // Common
  Language: '语言',
  Refresh: '刷新',
  'Refreshing…': '刷新中…',
  'Loading…': '加载中…',
  'Loading statistics…': '正在加载统计…',
  'Loading audit entries…': '正在加载审计记录…',
  'Loading mail entries…': '正在加载邮件记录…',
  'Loading servers…': '正在加载服务器…',
  'Loading join history…': '正在加载加入记录…',
  'Loading users…': '正在加载用户…',
  'Loading sessions…': '正在加载会话…',
  'Loading invites…': '正在加载邀请码…',
  'Loading skin…': '正在加载皮肤…',
  Copy: '复制',
  'Copy all': '全部复制',
  'Copy raw': '复制原始值',
  'Copy password': '复制密码',
  'Password copied': '密码已复制',
  'Copied': '已复制',
  '{label} copied': '{label}已复制',
  'Could not copy to the clipboard': '无法复制到剪贴板',
  'Could not copy the password': '无法复制密码',
  'Could not load server metadata': '无法加载服务器信息',
  Dismiss: '关闭',
  Close: '关闭',
  Search: '搜索',
  Filter: '筛选',
  Save: '保存',
  Delete: '删除',
  Revoke: '撤销',
  Enable: '启用',
  Disable: '禁用',
  Previous: '上一页',
  Next: '下一页',
  Actions: '操作',
  Status: '状态',
  Role: '角色',
  Created: '创建时间',
  Updated: '更新时间',
  Name: '名称',
  Address: '地址',
  Description: '描述',
  Email: '邮箱',
  Password: '密码',
  Username: '用户名',
  Note: '备注',
  All: '全部',
  Pending: '待处理',
  Failed: '失败',
  Sent: '已发送',
  Retry: '重试',
  Time: '时间',
  Actor: '操作者',
  Action: '动作',
  Detail: '详情',
  Limit: '数量',
  'Unknown client': '未知客户端',
  'no email': '无邮箱',
  'No description': '无描述',
  'This is you': '这是你',
  'Read only': '只读',
  'verified': '已验证',
  'pending': '待验证',
  'No entries.': '暂无记录。',
  'just now': '刚刚',
  '{count}m ago': '{count} 分钟前',
  '{count}h ago': '{count} 小时前',
  '{count}d ago': '{count} 天前',

  // Roles and statuses
  user: '玩家',
  moderator: '审核员',
  admin: '管理员',
  superadmin: '超级管理员',
  active: '正常',
  disabled: '已禁用',
  current: '当前',
  sent: '已发送',
  failed: '失败',
  locked: '已锁定',
  revoked: '已撤销',
  expired: '已过期',
  exhausted: '已用尽',

  // Navigation and app shell
  Dashboard: '仪表盘',
  Sessions: '会话',
  Profile: '个人资料',
  Admin: '管理',
  Staff: '工作人员',
  'Sign in': '登录',
  'Create account': '创建账号',
  'Sign out': '退出登录',
  'Skip to content': '跳到主要内容',
  'UMC Auth · community-operated Minecraft account service':
    'UMC Auth · 社区运营的 Minecraft 账号服务',
  'UMC Auth home': 'UMC Auth 首页',
  Main: '主导航',
  'Loading your session…': '正在加载会话…',
  'Admin overview': '管理概览',
  'Users': '用户',
  'Invite codes': '邀请码',
  'Audit': '审计',
  'Game servers': '游戏服务器',
  'Mail': '邮件',

  // Route titles
  'Verify email': '验证邮箱',
  Verify: '验证',
  Administration: '管理',
  'Choose a new password': '设置新密码',
  'Not found': '页面不存在',
  'Mail outbox': '发件箱',
  'Audit log': '审计日志',

  // Login
  'Welcome back': '欢迎回来',
  'Sign in with your username or email to continue.': '使用用户名或邮箱登录以继续。',
  'Email or username': '邮箱或用户名',
  'name@example.com': 'name@example.com',
  '••••••••': '••••••••',
  Show: '显示',
  Hide: '隐藏',
  'Forgot password?': '忘记密码？',
  'Signing in…': '登录中…',
  "Don't have an account?": '还没有账号？',
  'Create one': '立即注册',
  'Email verified. You can sign in now.': '邮箱已验证，现在可以登录。',
  'Password updated. Sign in with your new password.': '密码已更新，请使用新密码登录。',

  // Register
  'Enter your details to get started.': '填写以下信息开始使用。',
  'Registration is closed. Ask an administrator for an account.':
    '注册已关闭，请联系管理员获取账号。',
  'This is the first account. It becomes the': '这是第一个账号，将成为',
  'and is verified immediately.': '并立即完成验证。',
  '3–16 letters, digits, or underscores.': '3–16 个字母、数字或下划线。',
  'We will send a six-digit verification code.': '我们将发送六位验证码。',
  'username': '用户名',
  'At least 8 characters': '至少 8 个字符',
  'Confirm password': '确认密码',
  'Re-enter your password': '再次输入密码',
  'Invite code': '邀请码',
  'Bootstrap token': '初始化令牌',
  'Only needed while the instance has no accounts.': '仅在实例还没有账号时需要。',
  'This server protects registration with proof-of-work. Your browser solves a short challenge when you submit the form.':
    '本服务器使用工作量证明保护注册。提交表单时浏览器会自动计算一个短挑战。',
  'Human check': '人机验证',
  'Already have an account?': '已有账号？',
  'Creating account…': '创建账号中…',
  'Solving the proof-of-work challenge…': '正在计算工作量证明…',
  'Username must be 3–16 characters using letters, digits, or underscores.':
    '用户名必须为 3–16 个字母、数字或下划线。',
  'Password must be 8–128 characters.': '密码长度必须为 8–128 个字符。',
  'Passwords do not match.': '两次输入的密码不一致。',
  'Enter a valid email address.': '请输入有效的邮箱地址。',
  'Complete the Cloudflare Turnstile challenge.': '请完成 Cloudflare Turnstile 验证。',
  'Proof-of-work requires a secure browser context': '工作量证明需要安全的浏览器环境',
  'Could not load Cloudflare Turnstile': '无法加载 Cloudflare Turnstile',
  'That username or email is already registered.': '该用户名或邮箱已被注册。',
  'The invite code is missing, invalid, or expired.':
    '邀请码缺失、无效或已过期。',
  'The bootstrap token is invalid.': '初始化令牌无效。',
  'CAPTCHA verification failed. Try again.': '人机验证失败，请重试。',
  'Registration is closed on this server.': '本服务器已关闭注册。',
  'Email delivery is not configured. Contact the server operator.':
    '邮件投递未配置，请联系服务器管理员。',

  // Email verification
  'Email verification': '邮箱验证',
  'Check your inbox': '查看邮箱',
  'Enter the six-digit code we sent to your email address.':
    '输入我们发送到你邮箱的六位验证码。',
  'Verification code': '验证码',
  'Verifying…': '验证中…',
  'Resend code': '重新发送验证码',
  'Sending…': '发送中…',
  'Resend in {seconds}s': '{seconds} 秒后重发',
  'Return to sign in': '返回登录',
  'The code is invalid or expired. Request a new one.':
    '验证码无效或已过期，请重新获取。',
  'If this address needs verification, a new code was sent.':
    '如果该邮箱需要验证，新的验证码已发送。',
  'Wait a minute before requesting another code.': '请等待一分钟后再获取验证码。',
  'Email verified. Welcome.': '邮箱验证成功，欢迎。',

  // Password recovery
  'Account recovery': '账号找回',
  'Reset your password': '重置密码',
  'We will send a six-digit reset code if the address exists.':
    '如果该邮箱存在，我们会发送六位重置验证码。',
  'If the address exists, a reset code was sent. It expires in 10 minutes.':
    '如果该邮箱存在，重置验证码已发送，10 分钟内有效。',
  'Enter the reset code': '输入重置验证码',
  'Send reset code': '发送重置验证码',
  'Create an account': '创建账号',
  'Enter the code from your email and pick a new password.':
    '输入邮件中的验证码并设置新密码。',
  'Reset code': '重置验证码',
  'New password': '新密码',
  'Confirm new password': '确认新密码',
  'Updating…': '更新中…',
  'Update password': '更新密码',
  'Request a new code': '重新获取验证码',

  // Not found
  'Page not found': '页面不存在',
  'That page does not exist or you do not have access to it.':
    '该页面不存在，或你无权访问。',
  'Return to dashboard': '返回仪表盘',

  // Dashboard
  'Your account': '你的账号',
  'Welcome back, {name}': '欢迎回来，{name}',
  'This account is used for the website, your launcher, and the Minecraft server.':
    '此账号用于网站、启动器和 Minecraft 服务器。',
  'email verified': '邮箱已验证',
  'email not verified': '邮箱未验证',
  Account: '账号',
  'View profile': '查看资料',
  'Manage sessions': '管理会话',
  'Minecraft identity': 'Minecraft 身份',
  'Use this UUID when adding the player to a server whitelist or admin list.':
    '将玩家加入服务器白名单或管理员列表时使用此 UUID。',
  'Launcher endpoint': '启动器认证地址',
  'Give this Yggdrasil URL to authlib-injector or a launcher that supports external authentication.':
    '将此 Yggdrasil 地址填入 authlib-injector 或支持外置登录的启动器。',
  'Yggdrasil URL': 'Yggdrasil 地址',
  'Staff tools': '工作人员工具',
  'Manage accounts, roles, audit records, and the mail outbox.':
    '管理账号、角色、审计记录和发件箱。',
  'Review accounts and the audit log.': '查看账号和审计日志。',
  'Open admin': '打开管理后台',
  'Player': '玩家',
  'Moderator': '审核员',
  'Administrator': '管理员',
  'Superadmin': '超级管理员',

  // Sessions
  Security: '安全',
  'Devices and clients that can access your account.':
    '可以访问你账号的设备和客户端。',
  'Revoke other sessions': '撤销其他会话',
  'No active sessions.': '没有活动会话。',
  Client: '客户端',
  'Last used': '最后使用',
  Expires: '过期时间',
  'Revoke the {client} session?': '撤销 {client} 会话？',
  'Session revoked.': '会话已撤销。',
  'Signed out.': '已退出登录。',
  'Revoke {count} other session(s)?': '撤销其他 {count} 个会话？',
  '{count} session(s) could not be revoked.': '{count} 个会话撤销失败。',
  'Other sessions revoked.': '其他会话已撤销。',
  unknown: '未知',

  // Profile
  'Your identity across the website, launcher, and game server.':
    '你在网站、启动器和游戏服务器中的身份。',
  Identity: '身份',
  'Email status': '邮箱状态',
  'not verified': '未验证',
  'Minecraft profile': 'Minecraft 档案',
  UUID: 'UUID',
  'The launcher receives a signed textures property when it joins a server.':
    '启动器加入服务器时会收到带签名的 textures 属性。',
  Skin: '皮肤',
  'No skin uploaded.': '尚未上传皮肤。',
  'Skin file': '皮肤文件',
  '64×64 or 64×32 PNG, up to 1 MB.': '64×64 或 64×32 PNG，最大 1 MB。',
  Model: '模型',
  classic: '经典',
  slim: '纤细',
  'Applied on the next skin upload.': '在下次上传皮肤时生效。',
  'Remove skin': '删除皮肤',
  'Current skin': '当前皮肤',
  'Remove your current skin?': '删除当前皮肤？',
  'Skin updated.': '皮肤已更新。',
  'Skin removed.': '皮肤已删除。',
  Cape: '披风',
  'No cape uploaded.': '尚未上传披风。',
  'Cape file': '披风文件',
  '64×32 or 64×64 PNG, up to 1 MB.': '64×32 或 64×64 PNG，最大 1 MB。',
  'Remove cape': '删除披风',
  'Current cape': '当前披风',
  'Remove your current cape?': '删除当前披风？',
  'Cape updated.': '披风已更新。',
  'Cape removed.': '披风已删除。',
  'Account details': '账号详情',
  Session: '会话',
  'Account changes': '账号变更',
  'Email changes and self-service account deletion are not available yet. Contact a server administrator if you need help with account recovery.':
    '暂不支持修改邮箱或自助删除账号。如需账号找回帮助，请联系服务器管理员。',
  'Review sessions': '查看会话',
  'Reset password': '重置密码',
  'Profile refreshed.': '资料已刷新。',

  // Admin navigation / overview
  Overview: '概览',
  Invites: '邀请码',
  Servers: '服务器',
  'Signed in as {username} · {role}': '当前登录：{username} · {role}',
  'Total accounts': '账号总数',
  'Active': '正常',
  'Pending verification': '待验证',
  'Disabled': '已禁用',
  'Queued mail': '待发送邮件',
  Manage: '管理',
  'Account status, roles, verification, password resets, and audit records.':
    '管理账号状态、角色、验证、密码重置和审计记录。',
  'Recent activity': '最近活动',
  'No audit entries yet.': '暂无审计记录。',

  // Admin users
  'Search accounts, change roles, disable users, and reset credentials.':
    '搜索账号、修改角色、禁用用户和重置凭据。',
  'Username, email, or UUID': '用户名、邮箱或 UUID',
  'Search users': '搜索用户',
  'New password for': '新密码，账号：',
  'Shown once. Copy it now.': '只显示一次，请立即复制。',
  'No accounts matched this search.': '没有匹配的账号。',
  'Verified': '已验证',
  'Account disabled.': '账号已禁用。',
  'Account enabled.': '账号已启用。',
  'Role updated.': '角色已更新。',
  'Email marked verified.': '邮箱已标记为验证通过。',
  'Password reset. Copy the generated password now.':
    '密码已重置，请立即复制生成的新密码。',
  'Disable {username}?': '禁用 {username}？',
  'Enable {username}?': '启用 {username}？',
  'Mark {email} as verified?': '将 {email} 标记为已验证？',
  'Generate a new password for {username}?': '为 {username} 生成新密码？',
  'Role for {username}': '{username} 的角色',
  '{count} account(s) · page {page} of {pages}':
    '共 {count} 个账号 · 第 {page}/{pages} 页',

  // Admin audit
  'Recent administrative and security-relevant actions.':
    '最近的管理和安全相关操作。',
  'Actor, action, or detail': '操作者、动作或详情',
  'Search audit log': '搜索审计日志',
  'No audit entries matched this search.': '没有匹配的审计记录。',

  // Admin mail
  'Verification and password reset messages queued for delivery.':
    '排队等待投递的邮箱验证和密码重置邮件。',
  Recipient: '收件人',
  Kind: '类型',
  Attempts: '尝试次数',
  'Next attempt': '下次尝试',
  'Last error': '最后错误',
  'No mail entries.': '暂无邮件记录。',
  'Message queued for retry.': '邮件已加入重试队列。',

  // Admin servers
  'Registered Minecraft servers and recent Yggdrasil join activity. Only superadmins can change this list.':
    '已登记的 Minecraft 服务器和最近的 Yggdrasil 加入活动。只有超级管理员可以修改此列表。',
  'Add a server': '添加服务器',
  'Survival': '生存服',
  'mc.example.com:25565': 'mc.example.com:25565',
  'Main survival world': '主生存世界',
  'Create server': '创建服务器',
  'Creating…': '创建中…',
  'Registered servers': '已登记服务器',
  'No servers registered yet.': '尚未登记服务器。',
  Server: '服务器',
  'Server created.': '服务器已创建。',
  'Server enabled.': '服务器已启用。',
  'Server disabled.': '服务器已禁用。',
  'Server deleted.': '服务器已删除。',
  'Enable {name}?': '启用 {name}？',
  'Disable {name}?': '禁用 {name}？',
  'Delete {name}? Join history is kept.': '删除 {name}？加入记录会保留。',
  'Join history': '加入记录',
  'Recorded when a launcher authenticates against a game server. Filter by the server id sent in the join request.':
    '启动器向游戏服务器认证时会记录。可按加入请求中的服务器 id 筛选。',
  'Server id (optional)': '服务器 id（可选）',
  'Filter join history by server id': '按服务器 id 筛选加入记录',
  'No join records yet.': '暂无加入记录。',
  'Server id': '服务器 id',
  IP: 'IP',

  // Admin invites
  'Create registration invites for invite-only mode. Full codes are shown once after creation.':
    '为邀请码注册模式创建邀请码。完整邀请码只在创建后显示一次。',
  'New invite codes': '新的邀请码',
  'Shown once.': '只显示一次。',
  'Shown once. Copy them now.': '只显示一次，请立即复制。',
  'Create invites': '创建邀请码',
  Count: '数量',
  'Uses per code': '每个邀请码可用次数',
  '1–50 codes per batch.': '每批 1–50 个邀请码。',
  'Valid for days': '有效天数',
  'Optional batch note': '可选的批次备注',
  'Recent invites': '最近的邀请码',
  'No invite codes yet.': '暂无邀请码。',
  Code: '邀请码',
  Uses: '使用次数',
  'invite code': '邀请码',
  '{count} invite code(s) created.': '已创建 {count} 个邀请码。',
  'Revoke invite {prefix}?': '撤销邀请码 {prefix}？',
  'Invite revoked.': '邀请码已撤销。',

  // API errors
  'This host only serves the UMC Auth API': '此域名仅提供 UMC Auth API',
  'invite.create': '创建邀请码',
  'invite.revoke': '撤销邀请码',
  'mail.retry': '重试邮件',
  'profile.cape_remove': '删除披风',
  'profile.cape_update': '更新披风',
  'profile.skin_remove': '删除皮肤',
  'profile.skin_update': '更新皮肤',
  'server.create': '创建服务器',
  'server.delete': '删除服务器',
  'server.disable': '禁用服务器',
  'server.enable': '启用服务器',
  'user.disable': '禁用账号',
  'user.enable': '启用账号',
  'user.password_forgot': '申请重置密码',
  'user.password_reset': '重置密码',
  'user.password_reset_manual': '管理员重置密码',
  'user.register': '注册账号',
  'user.role_change': '修改角色',
  'user.verification_resend': '重发验证码',
  'user.verify_email': '验证邮箱',
  'user.verify_email_manual': '管理员验证邮箱',
  register: '注册验证',
  password_reset: '密码重置',
  test: '测试',
}

const errors = {
  invalid_json: '请求格式不正确。',
  invalid_username: '用户名格式不正确。',
  invalid_email: '邮箱格式不正确。',
  weak_password: '密码强度不足。',
  invalid_code: '验证码无效或已过期。',
  invalid_credentials: '用户名、邮箱或密码错误。',
  unauthorized: '登录状态已失效，请重新登录。',
  invalid_grant: '会话已失效，请重新登录。',
  forbidden: '没有权限执行此操作。',
  account_disabled: '账号已被禁用。',
  account_locked: '登录尝试过多，请稍后再试。',
  email_not_verified: '请先验证邮箱。',
  registration_conflict: '用户名或邮箱已被注册。',
  registration_closed: '当前已关闭注册。',
  invite_required: '需要邀请码。',
  invalid_invite: '邀请码无效或已过期。',
  bootstrap_required: '初始化令牌无效。',
  captcha_failed: '人机验证失败。',
  email_unavailable: '邮件服务未配置。',
  too_many_requests: '请求过于频繁，请稍后再试。',
  not_found: '请求的资源不存在。',
  server_conflict: '同名服务器已存在。',
  invalid_texture: '图片不是有效的 PNG，或尺寸不符合要求。',
  invalid_model: '皮肤模型必须是 classic 或 slim。',
  resend_too_soon: '请稍后再获取验证码。',
  request_failed: '请求失败。',
  network_error: '无法连接认证服务。',
  challenge_failed: '无法获取工作量证明挑战。',
  internal_error: '服务器内部错误，请稍后再试。',
}

function detectLocale() {
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY)
    if (supported.includes(saved)) return saved
  } catch {
    // Storage can be unavailable in private windows.
  }
  const language = (navigator.language || 'en').toLowerCase()
  return language.startsWith('zh') ? 'zh' : 'en'
}

export const locale = ref(detectLocale())

function applyDocumentLanguage() {
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale.value === 'zh' ? 'zh-CN' : 'en'
  }
}

applyDocumentLanguage()

export function setLocale(next) {
  if (!supported.includes(next)) return
  locale.value = next
  try {
    window.localStorage.setItem(STORAGE_KEY, next)
  } catch {
    // Ignore storage failures.
  }
  applyDocumentLanguage()
}

export function t(text, params) {
  if (typeof text !== 'string') return text
  let template = text
  if (locale.value === 'zh') {
    template = zh[text] ?? text
  }
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (match, name) => {
    return params[name] === undefined ? match : String(params[name])
  })
}

export function apiErrorText(code, fallback) {
  if (locale.value === 'zh' && code && errors[code]) return errors[code]
  return fallback
}

export const languages = [
  { value: 'en', label: 'EN' },
  { value: 'zh', label: '中文' },
]
