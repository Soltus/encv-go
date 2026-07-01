/**
 * 文件搜索 E2E 测试 — 复现"顶层 /d 搜不到文件"的 bug
 *
 * Bug 描述：
 *   用户报告：文件首页（/d，挂载点列表页）搜索搜不到内容，
 *   进入挂载点（/d/primary）才能搜到。
 *
 * 根因：
 *   前端 performSearch() 优先调用 /api/search/files（向量搜索端点），
 *   而该端点直接调用 s.mobileSvc.SearchFiles(path, query, recursive)，
 *   没有处理 /d 的多挂载点遍历逻辑（searchAcrossAllMounts）。
 *   当 path=/d 时，SearchFiles 把 /d 当物理路径，搜不到任何文件。
 *
 *   对比：/api/files/search 端点有 isMountRoot 判断，
 *   会调用 searchAcrossAllMounts 遍历所有挂载点。
 *
 * 修复标准：
 *   /api/search/files 端点在 path=/d 时也必须遍历所有挂载点，
 *   行为与 /api/files/search 一致。
 */

describe('文件搜索 — 顶层 /d 搜索复现', () => {
  const API_BASE = Cypress.env('apiBase') || 'http://localhost:2025'

  // ==========================================================================
  // 辅助函数
  // ==========================================================================

  function searchFilesAPI(path: string, keyword: string, recursive = false) {
    return cy.request({
      method: 'GET',
      url: `${API_BASE}/api/files/search`,
      qs: { path, keyword, recursive: String(recursive) },
      failOnStatusCode: false,
    }).then((res) => res.body)
  }

  function vectorSearchFilesAPI(path: string, query: string, recursive = false, limit = 200) {
    return cy.request({
      method: 'GET',
      url: `${API_BASE}/api/search/files`,
      qs: { q: query, path, recursive: String(recursive), limit: String(limit) },
      failOnStatusCode: false,
    }).then((res) => res.body)
  }

  function listFiles(path: string) {
    return cy.request({
      method: 'GET',
      url: `${API_BASE}/api/files`,
      qs: { path },
      failOnStatusCode: false,
    }).then((res) => res.body)
  }

  // ==========================================================================
  // 前置条件：确认测试环境有挂载点和文件
  // ==========================================================================

  describe('前置条件验证', () => {
    it('应能获取挂载点列表', () => {
      listFiles('/d').then((body) => {
        const files = body.files || []
        cy.log(`挂载点列表: ${files.length} 个`)
        files.forEach((f: any) => {
          cy.log(`  - ${f.name} (isDir=${f.isDirectory}) path=${f.path}`)
        })
        expect(files.length).to.be.greaterThan(0, '至少应有 1 个挂载点')
      })
    })

    it('挂载点内应有文件可搜索', () => {
      // 先列出挂载点
      listFiles('/d').then((body) => {
        const mounts = (body.files || []).filter((f: any) => f.isDirectory)
        if (mounts.length === 0) return

        const firstMount = mounts[0]
        cy.log(`检查挂载点: ${firstMount.name} (${firstMount.path})`)

        // 递归搜索该挂载点下所有文件
        return searchFilesAPI(firstMount.path, '', true).then((searchBody) => {
          const files = searchBody.files || []
          cy.log(`挂载点 ${firstMount.name} 下文件数: ${files.length}`)
          // 至少应该有文件可以搜索
          if (files.length === 0) {
            cy.log('⚠️ 挂载点下没有文件，后续测试可能无法复现 bug')
          }
        })
      })
    })
  })

  // ==========================================================================
  // 核心 bug 复现：/api/search/files 端点在 /d 搜索
  // ==========================================================================

  describe('Bug 复现：/api/search/files 在 /d 搜索', () => {
    it('Bug: /api/search/files path=/d 递归搜索应返回结果', () => {
      // 先获取挂载点下有文件可搜
      let searchKeyword = 'test'
      let foundInMount = false

      listFiles('/d').then((body) => {
        const mounts = (body.files || []).filter((f: any) => f.isDirectory)

        // 遍历挂载点，找一个能搜到文件的挂载点
        const mountPromises = mounts.map((m: any) => {
          return searchFilesAPI(m.path, '', true).then((searchBody) => {
            const files = searchBody.files || []
            if (files.length > 0) {
              foundInMount = true
              // 取第一个文件名的一部分作为搜索关键词
              const firstName = files[0].name
              // 取文件名前 4 个字符（避免特殊字符问题）
              searchKeyword = firstName.substring(0, Math.min(4, firstName.length))
              cy.log(`选定关键词: "${searchKeyword}" 来自文件: ${firstName} (挂载点: ${m.name})`)
            }
            return { mount: m.name, fileCount: files.length }
          })
        })

        return cy.wrap(Promise.all(mountPromises)).then((results: any) => {
          results.forEach((r: any) => {
            cy.log(`  挂载点 ${r.mount}: ${r.fileCount} 个文件`)
          })
        })
      }).then(() => {
        if (!foundInMount) {
          cy.log('⚠️ 所有挂载点都没有文件，跳过搜索测试')
          return
        }

        cy.log(`=== 用关键词 "${searchKeyword}" 在 /d 搜索 ===`)

        // 测试 /api/files/search（文件搜索端点）— 应该能搜到
        searchFilesAPI('/d', searchKeyword, true).then((body) => {
          const filesSearchResults = body.files || []
          cy.log(`/api/files/search /d recursive=true: ${filesSearchResults.length} 条结果`)
          filesSearchResults.forEach((f: any, i: number) => {
            if (i < 10) cy.log(`  files/search: ${f.name} -> ${f.path}`)
          })
        })

        // 测试 /api/search/files（向量搜索端点）— 这是前端实际调用的
        vectorSearchFilesAPI('/d', searchKeyword, true, 200).then((body) => {
          const vectorSearchResults = body.files || []
          cy.log(`/api/search/files /d recursive=true: ${vectorSearchResults.length} 条结果`)
          vectorSearchResults.forEach((f: any, i: number) => {
            if (i < 10) cy.log(`  search/files: ${f.name} -> ${f.path}`)
          })

          // 这就是 bug 所在：向量搜索端点在 /d 搜索时返回 0 结果
          // 因为它直接调用 SearchFiles("/d", ...) 没有遍历挂载点
          if (vectorSearchResults.length === 0) {
            cy.log('❌ BUG 复现：/api/search/files 在 /d 搜索返回 0 结果！')
            cy.log('   前端 performSearch() 优先调用此端点，所以用户在首页搜不到任何文件')
            cy.log('   根因：handleVectorSearchFilesGin 没有处理 /d 的多挂载点逻辑')
          }

          // 修复后这个断言应该通过
          expect(vectorSearchResults.length, '/api/search/files 在 /d 搜索应返回结果').to.be.greaterThan(0)
        })
      })
    })

    it('Bug: /api/search/files path=/d 非递归搜索应返回挂载点根目录文件', () => {
      // 非递归搜索 /d，应该返回各挂载点根目录的文件
      vectorSearchFilesAPI('/d', '', false, 200).then((body) => {
        const results = body.files || []
        cy.log(`/api/search/files /d recursive=false: ${results.length} 条结果`)
      })

      // 先找一个挂载点根目录下的文件名
      listFiles('/d').then((body) => {
        const mounts = (body.files || []).filter((f: any) => f.isDirectory)
        if (mounts.length === 0) return

        // 列出第一个挂载点的根目录
        return listFiles(mounts[0].path).then((mountBody) => {
          const rootFiles = (mountBody.files || []).filter((f: any) => !f.isDirectory)
          if (rootFiles.length === 0) {
            cy.log('⚠️ 挂载点根目录没有文件，跳过')
            return
          }

          const targetName = rootFiles[0].name
          const keyword = targetName.substring(0, Math.min(4, targetName.length))
          cy.log(`非递归搜索关键词: "${keyword}" 来自文件: ${targetName}`)

          // 向量搜索端点非递归 /d 搜索
          return vectorSearchFilesAPI('/d', keyword, false, 200).then((body) => {
            const results = body.files || []
            cy.log(`向量搜索 /d 非递归: ${results.length} 条结果`)

            if (results.length === 0) {
              cy.log('❌ BUG 复现：/api/search/files 在 /d 非递归搜索返回 0 结果！')
            }
          })
        })
      })
    })
  })

  // ==========================================================================
  // 对比测试：/api/files/search vs /api/search/files
  // ==========================================================================

  describe('端点对比：两个搜索 API 行为应一致', () => {
    it('两个搜索端点在 /d 搜索应返回相同数量的结果', () => {
      // 先找挂载点下有文件可搜
      let searchKeyword = 'a'

      listFiles('/d').then((body) => {
        const mounts = (body.files || []).filter((f: any) => f.isDirectory)
        if (mounts.length === 0) return

        return searchFilesAPI(mounts[0].path, '', true).then((searchBody) => {
          const files = searchBody.files || []
          if (files.length > 0) {
            searchKeyword = files[0].name.substring(0, Math.min(3, files[0].name.length))
          }
          cy.log(`对比测试关键词: "${searchKeyword}"`)
        })
      }).then(() => {
        cy.log(`=== 对比测试：关键词 "${searchKeyword}" ===`)

        cy.request({
          method: 'GET',
          url: `${API_BASE}/api/files/search`,
          qs: { path: '/d', keyword: searchKeyword, recursive: 'true' },
          failOnStatusCode: false,
        }).then((res1) => {
          const filesSearchCount = (res1.body.files || []).length
          cy.log(`/api/files/search 结果数: ${filesSearchCount}`)

          cy.request({
            method: 'GET',
            url: `${API_BASE}/api/search/files`,
            qs: { q: searchKeyword, path: '/d', recursive: 'true', limit: '200' },
            failOnStatusCode: false,
          }).then((res2) => {
            const vectorSearchCount = (res2.body.files || []).length
            cy.log(`/api/search/files 结果数: ${vectorSearchCount}`)

            // 两个端点应该返回相同（或接近相同）数量的结果
            // 如果 /api/search/files 返回 0 而 /api/files/search 返回 >0，就是 bug
            if (filesSearchCount > 0 && vectorSearchCount === 0) {
              cy.log('❌ BUG 确认：/api/files/search 能搜到，/api/search/files 搜不到')
            }

            // 修复后两个端点都应该能搜到
            expect(filesSearchCount, '/api/files/search 应有结果').to.be.greaterThan(0)
            expect(vectorSearchCount, '/api/search/files 应有结果').to.be.greaterThan(0)
          })
        })
      })
    })
  })

  // ==========================================================================
  // 前端交互测试：用户在首页搜索
  // ==========================================================================

  describe('前端交互：首页搜索', () => {
    beforeEach(() => {
      cy.visit('/tabs/files')
      cy.wait(1000)
    })

    it('用户在首页（/d）搜索应能看到结果', () => {
      // 确认当前在 /d
      cy.get('ion-title').should('contain.text', '文件')

      // 确认搜索框存在
      cy.get('ion-searchbar').should('exist').and('be.visible')

      // 开启递归搜索开关（如果存在）
      cy.get('body').then(($body) => {
        if ($body.find('ion-toggle').length > 0) {
          cy.get('ion-toggle').click()
          cy.wait(300)
        }
      })

      // 先找到挂载点下有文件可搜
      listFiles('/d').then((body) => {
        const mounts = (body.files || []).filter((f: any) => f.isDirectory)
        if (mounts.length === 0) return

        return searchFilesAPI(mounts[0].path, '', true).then((searchBody) => {
          const files = searchBody.files || []
          if (files.length === 0) {
            cy.log('⚠️ 挂载点下没有文件，跳过前端交互测试')
            return
          }

          const keyword = files[0].name.substring(0, Math.min(4, files[0].name.length))
          cy.log(`前端搜索关键词: "${keyword}"`)

          // 输入搜索词
          cy.get('ion-searchbar').click()
          cy.get('ion-searchbar input').type(keyword)
          cy.wait(2000) // 等待防抖 + 搜索完成

          // 检查是否有搜索结果
          cy.get('ion-list').then(($list) => {
            const items = $list.find('ion-item')
            cy.log(`搜索结果列表项数: ${items.length}`)

            if (items.length === 0) {
              cy.log('❌ BUG 复现：前端在 /d 首页搜索没有任何结果！')
            }

            expect(items.length, '首页搜索应有结果').to.be.greaterThan(0)
          })
        })
      })
    })
  })
})
