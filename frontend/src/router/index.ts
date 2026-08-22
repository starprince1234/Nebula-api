import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
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

export const routes: RouteRecordRaw[] = [
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/register', component: Register, meta: { public: true } },
  { path: '/recovery', component: Recovery, meta: { public: true } },
  { path: '/teacher/activate', component: TeacherActivation, meta: { public: true } },
  { path: '/', component: AppLayout, children: [
    { path: '', component: { template: '<div />' } },
    { path: 'student/api-keys', name: 'student-api-keys', component: StudentKeys, meta: { role: 'student', title: '申请密钥' } },
    { path: 'student/models', name: 'student-models', component: StudentModels, meta: { role: 'student', title: '模型广场' } },
    { path: 'mentor/projects', name: 'mentor-projects', component: MentorProjects, meta: { role: 'mentor', title: '项目管理' } },
    { path: 'mentor/reviews', name: 'mentor-reviews', component: MentorReviews, meta: { role: 'mentor', title: '审核密钥' } },
    { path: 'teacher/organizations', name: 'teacher-organizations', component: TeacherOrganizations, meta: { role: 'teacher', title: '组织管理' } },
    { path: 'teacher/projects', name: 'teacher-projects', component: TeacherProjects, meta: { role: 'teacher', title: '项目管理' } },
    { path: 'teacher/providers', name: 'teacher-providers', component: TeacherProviders, meta: { role: 'teacher', title: '供应商管理' } },
    { path: 'teacher/models', name: 'teacher-models', component: TeacherModels, meta: { role: 'teacher', title: '模型管理' } },
    { path: 'teacher/key-reviews', name: 'teacher-key-reviews', component: TeacherKeyReviews, meta: { role: 'teacher', title: '审批密钥' } }
  ] }
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.bootstrap()
  if (to.meta.public) return auth.isAuthenticated ? '/' : true
  if (!auth.isAuthenticated) return { path: '/login', query: { redirect: to.fullPath } }
  if (to.path === '/') return auth.user?.role === 'teacher' ? '/teacher/key-reviews' : auth.user?.role === 'mentor' ? '/mentor/reviews' : '/student/api-keys'
  const role = to.meta.role as 'student' | 'mentor' | 'teacher' | undefined
  if (role && !auth.hasRole(role)) return '/'
  return true
})
export default router
