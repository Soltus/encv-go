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
    cy.viewport(1920, 1080);
    cy.mount(SimverseHome, {
      global: {
        plugins: [pinia, router],
      },
    });
  });

  it('should render the page title', () => {
    cy.get('ion-title').should('contain', 'SimVerse 模拟世界');
    cy.screenshot('simverse-home-ion-title.png', { capture: 'runner' });
  });

  it('should render the world overview card', () => {
    cy.get('.world-overview').should('exist');
    cy.screenshot('simverse-home-world-overview.png', { capture: 'runner' });
  });

  it('should render the enter world button', () => {
    cy.contains('进入世界').should('exist');
    cy.screenshot('simverse-home-enter-button.png', { capture: 'runner' });
  });

  it('should render quick action buttons', () => {
    cy.contains('编年史').should('exist');
    cy.contains('设置').should('exist');
    cy.contains('日志').should('exist');
    cy.screenshot('simverse-home-quick-actions.png', { capture: 'runner' });
  });

  it('should render world state with default values', () => {
    cy.get('.world-overview').should('contain', 'Tick: 0');
    cy.get('.world-overview').should('contain', 'NPC 数: 0');
    cy.screenshot('simverse-home-world-state.png', { capture: 'runner' });
  });

  it('should verify full page rendering with screenshot', () => {
    cy.wait(500);
    
    cy.get('ion-header').should('exist');
    cy.get('ion-toolbar').should('exist');
    cy.get('ion-title').should('contain', 'SimVerse 模拟世界');
    cy.get('ion-content').should('exist');
    cy.get('.world-overview').should('exist');
    cy.get('.enter-world-btn').should('exist');
    cy.get('.quick-actions').should('exist');
    
    cy.screenshot('simverse-home-full-page.png', { capture: 'fullPage' });
  });
});
