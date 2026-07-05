// 验证 SimVerseHome 组件的真实渲染
import SimverseHome from '@self/views/SimverseHome.vue';
import { createPinia } from 'pinia';
import { createRouter, createWebHistory } from 'vue-router';

// 创建 Pinia 实例
const pinia = createPinia();

// 创建简单的路由配置
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: { template: '<div>Home</div>' } },
    { path: '/world', component: { template: '<div>World</div>' } },
    { path: '/chronicle', component: { template: '<div>Chronicle</div>' } },
    { path: '/tabs/settings', component: { template: '<div>Settings</div>' } },
    { path: '/tabs/devlogs', component: { template: '<div>DevLogs</div>' } },
  ],
});

describe('SimverseHome Component Rendering', () => {
  beforeEach(() => {
    cy.mount(SimverseHome, {
      global: {
        plugins: [pinia, router],
      },
    });
  });

  it('should render the page title', () => {
    cy.get('ion-title').should('contain', 'SimVerse 模拟世界');
  });

  it('should render the world overview card', () => {
    cy.get('.world-overview').should('exist');
  });

  it('should render the enter world button', () => {
    cy.contains('进入世界').should('exist');
  });

  it('should render quick action buttons', () => {
    cy.contains('编年史').should('exist');
    cy.contains('设置').should('exist');
    cy.contains('日志').should('exist');
  });

  it('should render world state with default values', () => {
    // 由于 store 初始值为 0，应该显示默认值
    cy.get('.world-overview').should('contain', 'Tick: 0');
    cy.get('.world-overview').should('contain', 'NPC 数: 0');
  });
});
