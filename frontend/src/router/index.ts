import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../store/auth'
import Login from '../views/auth/Login.vue'
import Register from '../views/auth/Register.vue'
import Recovery from '../views/auth/Recovery.vue'
import TeacherActivation from '../views/auth/TeacherActivation.vue'
import StudentKeys from '../views/student/StudentKeys.vue'
import StudentModels from '../views/student/StudentModels.vue'
import MentorProjects from '../views/mentor/MentorProjects.vue'
import MentorReviews from '../views/mentor/MentorReviews.vue'
import TeacherOrganizations from '../views/teacher/TeacherOrganizations.vue'
import TeacherProjects from '../views/teacher/TeacherProjects.vue'
import TeacherProviders from '../views/teacher/TeacherProviders.vue'
import TeacherModels from '../views/teacher/TeacherModels.vue'
import TeacherKeyReviews from '../views/teacher/TeacherKeyReviews.vue'
import AppLayout from '../layouts/AppLayout.vue'

const router = createRouter({ history: createWebHistory(), routes: [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/register', component: Register, meta: { public: true } },
  { path: '/recovery', component: Recovery, meta: { public: true } },
  { path: '/teacher/activate', component: TeacherActivation, meta: { public: true } },
  { path: '/', component: AppLayout, children: [
    { path: '', component: { template: '<div />' } },
    { path: 'student/api-keys', name: '申请密钥', component: StudentKeys, meta: { role: 'student' } },
    { path: 'student/models', name: '模型广场', component: StudentModels, meta: { role: 'student' } },
    { path: 'mentor/projects', name: '项目管理', component: MentorProjects, meta: { role: 'mentor' } },
    { path: 'mentor/reviews', name: '审核密钥', component: MentorReviews, meta: { role: 'mentor' } },
    { path: 'teacher/organizations', name: '组织管理', component: TeacherOrganizations, meta: { role: 'teacher' } },
    { path: 'teacher/projects', name: '项目管理', component: TeacherProjects, meta: { role: 'teacher' } },
    { path: 'teacher/providers', name: '供应商管理', component: TeacherProviders, meta: { role: 'teacher' } },
    { path: 'teacher/models', name: '模型管理', component: TeacherModels, meta: { role: 'teacher' } },
    { path: 'teacher/key-reviews', name: '审批密钥', component: TeacherKeyReviews, meta: { role: 'teacher' } }
  ] }
] })

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.bootstrap()
  if (to.meta.public) return auth.isAuthenticated ? '/' : true
  if (!auth.isAuthenticated) return { path: '/login', query: { redirect: to.fullPath } }
  if (to.path === '/') return auth.user?.role === 'teacher' ? '/teacher/key-reviews' : auth.user?.role === 'mentor' ? '/mentor/reviews' : '/student/api-keys'
  const role = to.meta.role as string | undefined
  if (role && !auth.hasRole(role as never)) return '/'
  return true
})
export default router
