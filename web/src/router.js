import { createRouter, createWebHistory } from 'vue-router'
import { auth, bootstrap, isAdmin, isStaff } from './auth'
import { t } from './i18n'

import LoginView from './views/LoginView.vue'
import RegisterView from './views/RegisterView.vue'
import VerifyEmailView from './views/VerifyEmailView.vue'
import ForgotPasswordView from './views/ForgotPasswordView.vue'
import ResetPasswordView from './views/ResetPasswordView.vue'
import DashboardView from './views/DashboardView.vue'
import SessionsView from './views/SessionsView.vue'
import ProfileView from './views/ProfileView.vue'
import AdminDashboardView from './views/admin/AdminDashboardView.vue'
import AdminUsersView from './views/admin/AdminUsersView.vue'
import AdminAuditView from './views/admin/AdminAuditView.vue'
import AdminMailView from './views/admin/AdminMailView.vue'
import AdminServersView from './views/admin/AdminServersView.vue'
import AdminInvitesView from './views/admin/AdminInvitesView.vue'
import NotFoundView from './views/NotFoundView.vue'

const routes = [
  {
    path: '/',
    name: 'dashboard',
    component: DashboardView,
    meta: { requiresAuth: true, title: 'Dashboard' },
  },
  {
    path: '/login',
    name: 'login',
    component: LoginView,
    meta: { guestOnly: true, title: 'Sign in' },
  },
  {
    path: '/register',
    name: 'register',
    component: RegisterView,
    meta: { guestOnly: true, title: 'Create account' },
  },
  {
    path: '/verify-email',
    name: 'verify-email',
    component: VerifyEmailView,
    meta: { title: 'Verify email' },
  },
  {
    path: '/forgot-password',
    name: 'forgot-password',
    component: ForgotPasswordView,
    meta: { guestOnly: true, title: 'Reset password' },
  },
  {
    path: '/reset-password',
    name: 'reset-password',
    component: ResetPasswordView,
    meta: { guestOnly: true, title: 'Choose a new password' },
  },
  {
    path: '/sessions',
    name: 'sessions',
    component: SessionsView,
    meta: { requiresAuth: true, title: 'Sessions' },
  },
  {
    path: '/profile',
    name: 'profile',
    component: ProfileView,
    meta: { requiresAuth: true, title: 'Profile' },
  },
  {
    path: '/admin',
    name: 'admin',
    component: AdminDashboardView,
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Admin overview' },
  },
  {
    path: '/admin/users',
    name: 'admin-users',
    component: AdminUsersView,
    meta: { requiresAuth: true, requiresStaff: true, title: 'Users' },
  },
  {
    path: '/admin/audit',
    name: 'admin-audit',
    component: AdminAuditView,
    meta: { requiresAuth: true, requiresStaff: true, title: 'Audit log' },
  },
  {
    path: '/admin/invites',
    name: 'admin-invites',
    component: AdminInvitesView,
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Invite codes' },
  },
  {
    path: '/admin/servers',
    name: 'admin-servers',
    component: AdminServersView,
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Game servers' },
  },
  {
    path: '/admin/mail',
    name: 'admin-mail',
    component: AdminMailView,
    meta: { requiresAuth: true, requiresAdmin: true, title: 'Mail outbox' },
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: NotFoundView,
    meta: { title: 'Not found' },
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

router.beforeEach(async (to) => {
  if (!auth.ready) await bootstrap()

  if (to.meta.requiresAuth && !auth.user) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.guestOnly && auth.user) {
    return { name: 'dashboard' }
  }
  if (to.meta.requiresAdmin && !isAdmin.value) {
    return { name: 'dashboard' }
  }
  if (to.meta.requiresStaff && !isStaff.value) {
    return { name: 'dashboard' }
  }
  return true
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${t(to.meta.title)} · UMC Auth` : 'UMC Auth'
})

export default router
