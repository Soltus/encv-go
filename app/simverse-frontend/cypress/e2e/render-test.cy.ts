describe('SimverseHome 页面渲染测试 (E2E)', () => {
  beforeEach(() => {
    cy.viewport(1920, 1080);
  });

  it('应该能访问 dev server', () => {
    // 尝试访问首页
    cy.request({
      url: 'http://localhost:8200/',
      failOnStatusCode: false,
      timeout: 30000,
    }).then((resp) => {
      cy.log('Response status:', resp.status);
      cy.log('Response body length:', resp.body.length);
    });
    
    // 访问页面
    cy.visit('http://localhost:8200/', { timeout: 30000 });
    
    // 等待页面加载
    cy.wait(3000);
    
    // 检查 body 内容
    cy.document().then((doc) => {
      const body = doc.body;
      cy.log('Body innerHTML length:', body.innerHTML.length);
      cy.log('Body has ion-app:', body.querySelector('ion-app') !== null);
      cy.log('Body has ion-content:', body.querySelector('ion-content') !== null);
    });
    
    // 截图验证
    cy.screenshot('simverse-home-e2e.png', { capture: 'fullPage' });
  });

  it('应该包含标题元素', () => {
    cy.visit('http://localhost:8200/', { timeout: 30000 });
    cy.wait(2000);
    
    cy.get('ion-title').should('exist');
    cy.screenshot('ion-title-e2e.png', { capture: 'runner' });
  });
});