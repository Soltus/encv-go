/**
 * 快速控制台输出捕获测试
 * 运行方式: pnpm cypress:open 选择此文件，或直接运行
 */
describe('Console Capture Test', () => {
  const consoleLogs: string[] = [];
  const consoleErrors: string[] = [];

  beforeEach(() => {
    // 清除日志记录
    consoleLogs.length = 0;
    consoleErrors.length = 0;
    
    // 监听控制台
    cy.window().then((win) => {
      cy.stub(win.console, 'log').callsFake((...args) => {
        consoleLogs.push(args.map(a => String(a)).join(' '));
      });
      cy.stub(win.console, 'error').callsFake((...args) => {
        consoleErrors.push(args.map(a => String(a)).join(' '));
      });
      cy.stub(win.console, 'warn').callsFake((...args) => {
        consoleLogs.push('[WARN] ' + args.map(a => String(a)).join(' '));
      });
    });
  });

  it('visit app and capture console', () => {
    cy.visit('http://localhost:8100/');
    
    // 等待页面加载
    cy.wait(3000);
    
    // 输出控制台日志到 Cypress UI
    cy.then(() => {
      if (consoleLogs.length > 0) {
        console.log('===== CONSOLE LOGS =====');
        consoleLogs.forEach(log => console.log(log));
      }
      
      if (consoleErrors.length > 0) {
        console.log('===== CONSOLE ERRORS =====');
        consoleErrors.forEach(err => console.error(err));
      }
      
      if (consoleLogs.length === 0 && consoleErrors.length === 0) {
        console.log('===== NO CONSOLE OUTPUT =====');
      }
    });
    
    // 页面应该显示 ion-app
    cy.get('ion-app').should('exist');
  });
});