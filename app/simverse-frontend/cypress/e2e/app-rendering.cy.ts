// 验证 SimVerse 应用在浏览器中的真实渲染
describe('SimVerse Application Rendering', () => {
  beforeEach(() => {
    // 访问应用首页
    cy.visit('/');
  });

  it('should load the application', () => {
    cy.url().should('eq', 'http://localhost:8200/simverse-home');
  });

  it('should render the Ionic app root', () => {
    // 验证 Ionic 应用根元素存在
    cy.get('ion-app').should('exist');
  });

  it('should render the home page content', () => {
    // 验证首页标题
    cy.contains('SimVerse 模拟世界').should('be.visible');
  });

  it('should render the world overview section', () => {
    // 验证世界概览区域
    cy.get('.world-overview').should('exist');
  });

  it('should render the enter world button', () => {
    // 验证进入世界按钮
    cy.contains('进入世界').should('be.visible');
  });

  it('should render quick action buttons', () => {
    // 验证快速操作按钮
    cy.contains('编年史').should('exist');
    cy.contains('设置').should('exist');
    cy.contains('日志').should('exist');
  });
});
