import Files from '../../src/views/Files.vue'
import { eventBus } from '../../src/composables/useEventBus'
import type { FileItem } from '../../src/api/encv'

function mockStreamFiles(files: FileItem[]) {
  const body = files.map(f => `data: ${JSON.stringify(f)}\n\n`).join('') + 'data: [DONE]\n\n'
  cy.intercept('GET', '**/api/files/stream*', {
    statusCode: 200,
    headers: { 'Content-Type': 'text/event-stream' },
    body,
  }).as('streamFiles')
}

function mockPlugins() {
  cy.intercept('GET', '**/api/plugins*', {
    body: [],
    statusCode: 200,
  }).as('getPlugins')
}

function mockTags() {
  cy.intercept('GET', '**/api/tags*', {
    body: [],
    statusCode: 200,
  }).as('getTags')
}

function mockFileInfo(file: FileItem) {
  cy.intercept('GET', `**/api/file/info*path=*${encodeURIComponent(file.path)}*`, {
    body: file,
    statusCode: 200,
  }).as('getFileInfo')
}

function mockSearchFiles(results: FileItem[]) {
  cy.intercept('GET', '**/api/files/search*', {
    body: results,
    statusCode: 200,
  }).as('searchFiles')
}

function generateMockFiles(count: number, basePath = '/d'): FileItem[] {
  const files: FileItem[] = []
  for (let i = 0; i < count; i++) {
    const isDir = i % 5 === 0
    files.push({
      name: isDir ? `folder-${String(i).padStart(3, '0')}` : `file-${String(i).padStart(3, '0')}.txt`,
      path: `${basePath}/${isDir ? `folder-${String(i).padStart(3, '0')}` : `file-${String(i).padStart(3, '0')}.txt`}`,
      isDirectory: isDir,
      size: isDir ? 0 : 1024 * (i + 1),
      modified: new Date(Date.now() - i * 60000).toISOString(),
    })
  }
  return files
}

describe('Files tab 基础渲染', () => {
  it('smoke: mount 成功并加载文件列表', () => {
    const mockFiles = generateMockFiles(20)
    mockStreamFiles(mockFiles)
    mockPlugins()
    mockTags()
    cy.mount(Files as any)
    cy.wait(3000)
    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      expect(api).to.exist
      expect(api.getCurrentFiles).to.be.a('function')
      const files = api.getCurrentFiles()
      expect(files.length).to.equal(20)
    })
  })
})

describe('Files tab file:change 增量更新', () => {
  it('delete action: 文件从列表中移除', () => {
    const mockFiles = generateMockFiles(10)
    mockStreamFiles(mockFiles)
    mockPlugins()
    mockTags()
    cy.mount(Files as any)
    cy.wait(4000)

    const targetPath = '/d/file-001.txt'
    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const before = api.getCurrentFiles()
      cy.log(`Before delete: ${before.length} files`)
      cy.log(`Looking for: ${targetPath}`)
      const found = before.find((f: FileItem) => f.path === targetPath)
      cy.log(`Found: ${found ? found.name : 'NOT FOUND'}`)
      expect(found).to.exist
    })

    cy.then(() => {
      eventBus.emit('file:change', { path: targetPath, action: 'delete' })
    })
    cy.wait(1000)

    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const after = api.getCurrentFiles()
      cy.log(`After delete: ${after.length} files`)
      const found = after.find((f: FileItem) => f.path === targetPath)
      cy.log(`Still found: ${found ? found.name : 'NO (good)'}`)
      expect(found).to.not.exist
      expect(after.length).to.equal(9)
    })
  })

  it('create action: 新文件追加到列表', () => {
    const mockFiles = generateMockFiles(10)
    mockStreamFiles(mockFiles)
    mockPlugins()
    mockTags()

    const newFile: FileItem = {
      name: 'new-file.txt',
      path: '/d/new-file.txt',
      isDirectory: false,
      size: 9999,
      modified: new Date().toISOString(),
    }
    cy.intercept('GET', '**/api/file/info*', {
      body: newFile,
      statusCode: 200,
    }).as('getFileInfo')

    cy.mount(Files as any)
    cy.wait(3000)

    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const before = api.getCurrentFiles()
      expect(before.length).to.equal(10)
    })

    cy.then(() => {
      eventBus.emit('file:change', { path: '/d/new-file.txt', action: 'create' })
    })
    cy.wait(500)

    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const after = api.getCurrentFiles()
      expect(after.length).to.equal(11)
      expect(after.find((f: FileItem) => f.name === 'new-file.txt')).to.exist
    })
  })

  it('modify action: 文件 metadata 更新', () => {
    const mockFiles = generateMockFiles(10)
    mockStreamFiles(mockFiles)
    mockPlugins()
    mockTags()

    const modifiedFile: FileItem = {
      ...mockFiles[2],
      size: 99999,
      modified: new Date().toISOString(),
    }
    cy.intercept('GET', '**/api/file/info*', {
      body: modifiedFile,
      statusCode: 200,
    }).as('getFileInfo')

    cy.mount(Files as any)
    cy.wait(3000)

    const targetPath = mockFiles[2].path
    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const before = api.getCurrentFiles().find((f: FileItem) => f.path === targetPath)
      expect(before).to.exist
      expect(before.size).to.not.equal(99999)
    })

    cy.then(() => {
      eventBus.emit('file:change', { path: targetPath, action: 'modify' })
    })
    cy.wait(500)

    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const after = api.getCurrentFiles().find((f: FileItem) => f.path === targetPath)
      expect(after).to.exist
      expect(after.size).to.equal(99999)
    })
  })
})

describe('Files tab 频繁 file:change 防抖合并', () => {
  it('300ms 内多次 delete 合并为一次处理', () => {
    const mockFiles = generateMockFiles(20)
    mockStreamFiles(mockFiles)
    mockPlugins()
    mockTags()
    cy.mount(Files as any)
    cy.wait(3000)

    const paths = ['/d/file-001.txt', '/d/file-002.txt', '/d/file-003.txt']

    cy.window().then(() => {
      paths.forEach((p, i) => {
        setTimeout(() => {
          eventBus.emit('file:change', { path: p, action: 'delete' })
        }, i * 50)
      })
    })

    cy.wait(600)

    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const after = api.getCurrentFiles()
      expect(after.length).to.equal(17)
      paths.forEach(p => {
        expect(after.find((f: FileItem) => f.path === p)).to.not.exist
      })
    })
  })

  it('同一路径多次事件：latest action wins', () => {
    const mockFiles = generateMockFiles(10)
    mockStreamFiles(mockFiles)
    mockPlugins()
    mockTags()
    cy.mount(Files as any)
    cy.wait(3000)

    const targetPath = '/d/file-005.txt'

    cy.window().then(() => {
      eventBus.emit('file:change', { path: targetPath, action: 'delete' })
      setTimeout(() => {
        eventBus.emit('file:change', { path: targetPath, action: 'create' })
      }, 100)
    })

    const recreatedFile: FileItem = {
      name: 'file-005.txt',
      path: targetPath,
      isDirectory: false,
      size: 55555,
      modified: new Date().toISOString(),
    }
    cy.intercept('GET', '**/api/file/info*', {
      body: recreatedFile,
      statusCode: 200,
    })

    cy.wait(600)

    cy.window().then((win) => {
      const api = (win as any).__ENCV_TEST
      const files = api.getCurrentFiles()
      const f = files.find((x: FileItem) => x.path === targetPath)
      expect(f).to.exist
      expect(f.size).to.equal(55555)
    })
  })
})
