import { useMemo, useState } from 'react'
import {
  ArrowLeft,
  ArrowUpRight,
  Bell,
  BookOpen,
  Check,
  ChevronDown,
  ChevronRight,
  CircleUserRound,
  Code2,
  Copy,
  Download,
  FileCode2,
  FileText,
  GitBranch,
  Heart,
  Menu,
  MessageSquare,
  MoreHorizontal,
  Play,
  Search,
  Send,
  Share2,
  Sparkles,
  Star,
  Tag,
  Upload,
  X,
} from 'lucide-react'
import './App.css'

type Project = {
  id: string
  name: string
  slug: string
  summary: string
  description: string
  category: string
  tags: string[]
  stack: string[]
  license: string
  updated: string
  downloads: string
  stars: string
  comments: number
  maintainer: string
  initials: string
  accent: string
  status: string
  repo: string
}

type CommentItem = {
  id: number
  user: string
  initials: string
  time: string
  quote: string
  text: string
  status: 'open' | 'resolved'
}

const categories = ['全部项目', 'Multi-Agent', 'RAG Agent', 'Coding Agent', 'Workflow Agent', 'Agent Framework']

const projects: Project[] = [
  {
    id: 'atlas',
    name: 'Atlas Agent',
    slug: 'atlas-agent',
    summary: '面向复杂任务的多 Agent 协作运行时',
    description: '把规划、检索、执行和复盘拆成可观察的 Agent 节点，让复杂任务保持清晰、可控、可复用。',
    category: 'Multi-Agent',
    tags: ['多智能体', '工作流', '可观测'],
    stack: ['Python', 'LangGraph', 'OpenAI'],
    license: 'Apache-2.0',
    updated: '2 小时前',
    downloads: '12.8k',
    stars: '2.4k',
    comments: 18,
    maintainer: '北岛实验室',
    initials: 'B',
    accent: 'blue',
    status: '活跃维护',
    repo: 'github.com/atlas-lab/agent',
  },
  {
    id: 'paperclip',
    name: 'Paperclip RAG',
    slug: 'paperclip-rag',
    summary: '为团队知识库设计的轻量 RAG Agent',
    description: '从文档清洗、切分、检索到带引用回答，提供一条适合内部知识库的可视化链路。',
    category: 'RAG Agent',
    tags: ['知识库', '引用回答', '中文'],
    stack: ['TypeScript', 'LlamaIndex', 'Elasticsearch'],
    license: 'MIT',
    updated: '昨天',
    downloads: '8.1k',
    stars: '1.8k',
    comments: 12,
    maintainer: 'Paperclip',
    initials: 'P',
    accent: 'orange',
    status: '生产可用',
    repo: 'github.com/paperclip-ai/rag',
  },
  {
    id: 'forge',
    name: 'Forge Runner',
    slug: 'forge-runner',
    summary: '让 Coding Agent 安全执行真实工程任务',
    description: '提供沙箱、工具调用、补丁预览和测试回放，让编码 Agent 的每一步都能被开发者检查。',
    category: 'Coding Agent',
    tags: ['代码 Agent', '沙箱', '工具调用'],
    stack: ['Go', 'React', 'Docker'],
    license: 'MIT',
    updated: '3 天前',
    downloads: '5.6k',
    stars: '960',
    comments: 9,
    maintainer: 'Forge Team',
    initials: 'F',
    accent: 'green',
    status: '实验项目',
    repo: 'github.com/forge-runner/core',
  },
  {
    id: 'relay',
    name: 'Relay MCP',
    slug: 'relay-mcp',
    summary: '面向工具调用的 MCP 服务编排层',
    description: '把分散的工具能力整理成可发现、可授权、可审计的服务目录，降低 Agent 接入成本。',
    category: 'Agent Framework',
    tags: ['MCP', '工具调用', '服务治理'],
    stack: ['Go', 'MCP', 'Redis'],
    license: 'Apache-2.0',
    updated: '上周',
    downloads: '4.2k',
    stars: '742',
    comments: 7,
    maintainer: 'Relay Open',
    initials: 'R',
    accent: 'purple',
    status: '活跃维护',
    repo: 'github.com/relay-open/mcp',
  },
]

const initialComments: CommentItem[] = [
  {
    id: 1,
    user: '林默',
    initials: '林',
    time: '18 分钟前',
    quote: '每个节点都需要声明输入、输出和失败策略。',
    text: '这里是否可以补充一个最小节点示例？新用户会更容易理解。',
    status: 'open',
  },
  {
    id: 2,
    user: '苏打',
    initials: '苏',
    time: '昨天',
    quote: '规划器会将目标拆解为多个可执行步骤。',
    text: '这一段已经在快速开始中补充了示例，问题已解决。',
    status: 'resolved',
  },
]

const codeFiles = [
  { name: 'README.md', type: 'md', size: '8.4 KB' },
  { name: 'atlas', type: 'folder', size: '—' },
  { name: 'runtime.py', type: 'py', size: '12.6 KB' },
  { name: 'planner.py', type: 'py', size: '7.2 KB' },
  { name: 'config.example.yaml', type: 'yaml', size: '1.8 KB' },
]

function App() {
  const [activeTab, setActiveTab] = useState('探索')
  const [activeCategory, setActiveCategory] = useState('全部项目')
  const [search, setSearch] = useState('')
  const [selectedProject, setSelectedProject] = useState<Project | null>(null)
  const [detailTab, setDetailTab] = useState('文档阅读')
  const [saved, setSaved] = useState<string[]>([])
  const [comments, setComments] = useState(initialComments)
  const [draftComment, setDraftComment] = useState('')
  const [commentComposerOpen, setCommentComposerOpen] = useState(false)
  const [selectedQuote, setSelectedQuote] = useState('每个节点都需要声明输入、输出和失败策略。')
  const [toast, setToast] = useState('')
  const [loginOpen, setLoginOpen] = useState(false)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)

  const filteredProjects = useMemo(() => {
    const normalizedSearch = search.trim().toLowerCase()
    return projects.filter((project) => {
      const matchesCategory = activeCategory === '全部项目' || project.category === activeCategory
      const haystack = [project.name, project.summary, project.category, ...project.tags, ...project.stack]
        .join(' ')
        .toLowerCase()
      return matchesCategory && (!normalizedSearch || haystack.includes(normalizedSearch))
    })
  }, [activeCategory, search])

  const showToast = (message: string) => {
    setToast(message)
    window.setTimeout(() => setToast(''), 2400)
  }

  const openProject = (project: Project) => {
    setSelectedProject(project)
    setDetailTab('文档阅读')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  const toggleSaved = (projectId: string) => {
    setSaved((current) => (current.includes(projectId) ? current.filter((id) => id !== projectId) : [...current, projectId]))
    showToast(saved.includes(projectId) ? '已取消收藏' : '已收藏到个人中心')
  }

  const handleSelection = () => {
    const selection = window.getSelection()?.toString().trim()
    if (selection) {
      setSelectedQuote(selection)
      setCommentComposerOpen(true)
    }
  }

  const submitComment = () => {
    if (!draftComment.trim()) return
    setComments((current) => [
      {
        id: Date.now(),
        user: '我',
        initials: '我',
        time: '刚刚',
        quote: selectedQuote,
        text: draftComment.trim(),
        status: 'open',
      },
      ...current,
    ])
    setDraftComment('')
    setCommentComposerOpen(false)
    showToast('评论已发布，已同步到当前文档')
  }

  const resolveComment = (commentId: number) => {
    setComments((current) => current.map((comment) => (comment.id === commentId ? { ...comment, status: 'resolved' } : comment)))
    showToast('评论已标记为已解决')
  }

  return (
    <div className="app-shell">
      <header className="site-header">
        <button className="brand" onClick={() => setSelectedProject(null)} aria-label="返回首页">
          <span className="brand-mark">新</span>
          <span>
            <strong>新猿译码</strong>
            <small>AGENT OPEN SOURCE HUB</small>
          </span>
        </button>

        <nav className={`main-nav ${mobileMenuOpen ? 'is-open' : ''}`}>
          {['探索', '趋势', '最新更新', '社区'].map((item) => (
            <button key={item} className={activeTab === item ? 'active' : ''} onClick={() => { setActiveTab(item); setSelectedProject(null); setMobileMenuOpen(false) }}>
              {item}
            </button>
          ))}
        </nav>

        <div className="header-actions">
          <label className="global-search">
            <Search size={16} />
            <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索项目、文档或技术栈" />
            <kbd>⌘ K</kbd>
          </label>
          <button className="icon-button quiet" title="通知" aria-label="通知" onClick={() => showToast('暂时没有新的通知')}><Bell size={18} /></button>
          <button className="login-button" onClick={() => setLoginOpen(true)}><CircleUserRound size={16} /> 登录</button>
          <button className="icon-button mobile-only" title="打开菜单" aria-label="打开菜单" onClick={() => setMobileMenuOpen((open) => !open)}><Menu size={19} /></button>
        </div>
      </header>

      {selectedProject ? (
        <ProjectDetail
          project={selectedProject}
          detailTab={detailTab}
          setDetailTab={setDetailTab}
          isSaved={saved.includes(selectedProject.id)}
          onBack={() => setSelectedProject(null)}
          onToggleSaved={() => toggleSaved(selectedProject.id)}
          onShare={() => { navigator.clipboard?.writeText(window.location.href); showToast('项目链接已复制') }}
          onDownload={() => showToast('演示下载已开始')}
          comments={comments}
          selectedQuote={selectedQuote}
          commentComposerOpen={commentComposerOpen}
          setCommentComposerOpen={setCommentComposerOpen}
          draftComment={draftComment}
          setDraftComment={setDraftComment}
          onSelection={handleSelection}
          onSubmitComment={submitComment}
          onResolveComment={resolveComment}
          showToast={showToast}
        />
      ) : (
        <Home
          activeTab={activeTab}
          activeCategory={activeCategory}
          setActiveCategory={setActiveCategory}
          filteredProjects={filteredProjects}
          saved={saved}
          onOpenProject={openProject}
          onToggleSaved={toggleSaved}
        />
      )}

      {toast && <div className="toast"><Check size={16} /> {toast}</div>}
      {loginOpen && <LoginModal onClose={() => setLoginOpen(false)} onLogin={() => { setLoginOpen(false); showToast('演示登录成功') }} />}
    </div>
  )
}

function Home({
  activeTab,
  activeCategory,
  setActiveCategory,
  filteredProjects,
  saved,
  onOpenProject,
  onToggleSaved,
}: {
  activeTab: string
  activeCategory: string
  setActiveCategory: (category: string) => void
  filteredProjects: Project[]
  saved: string[]
  onOpenProject: (project: Project) => void
  onToggleSaved: (projectId: string) => void
}) {
  return (
    <main>
      <section className="hero-band">
        <div className="hero-copy">
          <span className="eyebrow"><span className="status-dot" /> 为 Agent 开发者整理的开源项目空间</span>
          <h1>找到下一次<br /><em>有用的连接。</em></h1>
          <p>浏览真实可用的 Agent 项目，阅读实现文档，查看关键代码，并把好的想法带回你的工作流。</p>
          <div className="hero-actions">
            <button className="primary-button" onClick={() => document.getElementById('project-grid')?.scrollIntoView({ behavior: 'smooth' })}>开始探索 <ArrowUpRight size={16} /></button>
            <button className="text-button" onClick={() => window.open('https://github.com', '_blank')}>提交开源项目 <Upload size={16} /></button>
          </div>
        </div>
        <div className="hero-visual" aria-label="Agent 工作流预览">
          <div className="visual-topline"><span>LIVE / PROJECT MAP</span><span>2026.07</span></div>
          <div className="signal-grid">
            <div className="signal-line line-one" /><div className="signal-line line-two" /><div className="signal-line line-three" />
            <div className="signal-node node-a"><span>R</span><small>RETRIEVE</small></div>
            <div className="signal-node node-b"><span>P</span><small>PLAN</small></div>
            <div className="signal-node node-c"><span>T</span><small>TOOL</small></div>
            <div className="signal-node node-d"><span>↗</span><small>OUTPUT</small></div>
          </div>
          <div className="visual-footer"><span>372 个公开项目</span><span className="visual-pulse">正在更新</span></div>
        </div>
      </section>

      <section className="metrics-strip">
        <div><strong>372</strong><span>公开项目</span></div>
        <div><strong>86</strong><span>今日更新</span></div>
        <div><strong>12.4k</strong><span>本周下载</span></div>
        <div><strong>98%</strong><span>项目可读</span></div>
        <div className="metrics-note"><Sparkles size={17} /><span>中文项目优先，欢迎分享你的 Agent 实验。</span></div>
      </section>

      <section className="content-section" id="project-grid">
        <div className="section-heading">
          <div><span className="section-kicker">PROJECT INDEX / 01</span><h2>{activeTab === '探索' ? '探索项目' : activeTab}</h2></div>
          <button className="outline-button">查看全部 <ChevronRight size={15} /></button>
        </div>
        <div className="filter-row">
          {categories.map((category) => <button key={category} className={`filter-chip ${activeCategory === category ? 'active' : ''}`} onClick={() => setActiveCategory(category)}>{category}</button>)}
          <button className="filter-more"><Tag size={14} /> 更多筛选 <ChevronDown size={14} /></button>
        </div>

        {filteredProjects.length ? (
          <div className="project-grid">
            {filteredProjects.map((project) => <ProjectCard key={project.id} project={project} isSaved={saved.includes(project.id)} onOpen={() => onOpenProject(project)} onToggleSaved={() => onToggleSaved(project.id)} />)}
          </div>
        ) : <div className="empty-state"><Search size={24} /><h3>没有找到匹配项目</h3><p>试试其他关键词或清空筛选条件。</p></div>}
      </section>

      <section className="editorial-band">
        <div><span className="section-kicker">FROM THE COMMUNITY / 02</span><h2>好的项目，值得被读懂。</h2><p>项目不只是一个仓库地址。我们希望让每个 Agent 的背景、设计选择和使用方式都能被清楚地留下来。</p></div>
        <div className="editorial-quote"><MessageSquare size={20} /><p>“把复杂的 Agent 工程，整理成可以被下一位开发者接住的知识。”</p><span>新猿译码编辑部</span></div>
      </section>

      <section className="content-section compact-section">
        <div className="section-heading"><div><span className="section-kicker">CURATED NOTE / 03</span><h2>本周精选</h2></div><button className="text-button">阅读编辑手记 <ArrowUpRight size={15} /></button></div>
        <div className="curated-row">
          <div className="curated-cover cover-blue"><div className="cover-label">FIELD NOTE 07</div><span>Agent<br />Observability</span></div>
          <div className="curated-copy"><span className="meta-label">编辑推荐 · 6 分钟阅读</span><h3>为什么 Agent 需要自己的“运行日志”？</h3><p>从单次回答到可回放的任务链路，观察每一步工具调用，才能让实验走向生产。</p><button className="text-button">打开文章 <ArrowUpRight size={15} /></button></div>
        </div>
      </section>
      <footer className="site-footer"><span>© 2026 新猿译码</span><span>一套面向 Agent 开发者的开放索引</span><span>Made for useful work.</span></footer>
    </main>
  )
}

function ProjectCard({ project, isSaved, onOpen, onToggleSaved }: { project: Project; isSaved: boolean; onOpen: () => void; onToggleSaved: () => void }) {
  return (
    <article className="project-card">
      <button className={`project-cover ${project.accent}`} onClick={onOpen} aria-label={`打开 ${project.name}`}>
        <div className="cover-orbit orbit-one" /><div className="cover-orbit orbit-two" /><div className="cover-orbit orbit-three" />
        <span className="cover-monogram">{project.name.slice(0, 1)}</span><span className="cover-index">0{projects.indexOf(project) + 1} / 04</span>
      </button>
      <div className="project-card-body">
        <div className="card-title-row"><div><span className="project-category">{project.category}</span><h3><button onClick={onOpen}>{project.name}</button></h3></div><button className={`icon-button ${isSaved ? 'saved' : ''}`} title={isSaved ? '取消收藏' : '收藏项目'} aria-label={isSaved ? '取消收藏' : '收藏项目'} onClick={onToggleSaved}><Heart size={17} fill={isSaved ? 'currentColor' : 'none'} /></button></div>
        <p>{project.summary}</p>
        <div className="tag-row">{project.tags.slice(0, 2).map((tag) => <span key={tag}>{tag}</span>)}</div>
        <div className="card-footer"><span><Star size={13} fill="currentColor" /> {project.stars}</span><span><Download size={13} /> {project.downloads}</span><span className="card-updated">{project.updated}</span></div>
      </div>
    </article>
  )
}

function ProjectDetail({
  project,
  detailTab,
  setDetailTab,
  isSaved,
  onBack,
  onToggleSaved,
  onShare,
  onDownload,
  comments,
  selectedQuote,
  commentComposerOpen,
  setCommentComposerOpen,
  draftComment,
  setDraftComment,
  onSelection,
  onSubmitComment,
  onResolveComment,
  showToast,
}: {
  project: Project
  detailTab: string
  setDetailTab: (tab: string) => void
  isSaved: boolean
  onBack: () => void
  onToggleSaved: () => void
  onShare: () => void
  onDownload: () => void
  comments: CommentItem[]
  selectedQuote: string
  commentComposerOpen: boolean
  setCommentComposerOpen: (open: boolean) => void
  draftComment: string
  setDraftComment: (value: string) => void
  onSelection: () => void
  onSubmitComment: () => void
  onResolveComment: (commentId: number) => void
  showToast: (message: string) => void
}) {
  return (
    <main className="detail-page">
      <div className="detail-topbar"><button className="back-button" onClick={onBack}><ArrowLeft size={16} /> 返回项目索引</button><span className="breadcrumb">/ {project.category} / {project.name}</span></div>
      <section className="detail-intro">
        <div className={`detail-mark ${project.accent}`}><span>{project.name.slice(0, 1)}</span></div>
        <div className="detail-copy"><span className="eyebrow">{project.status} · 最后更新于 {project.updated}</span><h1>{project.name}</h1><p>{project.description}</p><div className="detail-meta"><span><CircleUserRound size={15} /> {project.maintainer}</span><span><GitBranch size={15} /> {project.repo}</span><span><Tag size={15} /> {project.license}</span></div></div>
        <div className="detail-actions"><button className={`outline-button ${isSaved ? 'is-saved' : ''}`} onClick={onToggleSaved}><Heart size={15} fill={isSaved ? 'currentColor' : 'none'} /> {isSaved ? '已收藏' : '收藏'}</button><button className="icon-button" title="分享项目" aria-label="分享项目" onClick={onShare}><Share2 size={17} /></button><button className="icon-button" title="更多操作" aria-label="更多操作"><MoreHorizontal size={18} /></button></div>
      </section>
      <div className="detail-stats"><div><strong>{project.stars}</strong><span>Stars</span></div><div><strong>{project.downloads}</strong><span>下载</span></div><div><strong>{project.comments}</strong><span>讨论</span></div><div><strong>v0.8.2</strong><span>当前版本</span></div></div>
      <nav className="detail-tabs">{['项目概览', '文档阅读', '代码预览', '下载资源'].map((tab) => <button key={tab} className={detailTab === tab ? 'active' : ''} onClick={() => setDetailTab(tab)}>{tab}</button>)}</nav>

      {detailTab === '文档阅读' && <DocumentView project={project} comments={comments} selectedQuote={selectedQuote} commentComposerOpen={commentComposerOpen} setCommentComposerOpen={setCommentComposerOpen} draftComment={draftComment} setDraftComment={setDraftComment} onSelection={onSelection} onSubmitComment={onSubmitComment} onResolveComment={onResolveComment} />}
      {detailTab === '代码预览' && <CodeView project={project} onCopy={() => showToast('代码已复制到剪贴板')} />}
      {detailTab === '下载资源' && <DownloadView onDownload={onDownload} />}
      {detailTab === '项目概览' && <OverviewView project={project} onRead={() => setDetailTab('文档阅读')} />}
    </main>
  )
}

function DocumentView({ project, comments, selectedQuote, commentComposerOpen, setCommentComposerOpen, draftComment, setDraftComment, onSelection, onSubmitComment, onResolveComment }: { project: Project; comments: CommentItem[]; selectedQuote: string; commentComposerOpen: boolean; setCommentComposerOpen: (open: boolean) => void; draftComment: string; setDraftComment: (value: string) => void; onSelection: () => void; onSubmitComment: () => void; onResolveComment: (commentId: number) => void }) {
  return (
    <section className="doc-workspace">
      <aside className="doc-sidebar"><div className="sidebar-heading"><span>文档目录</span><button className="icon-button quiet" title="收起目录" aria-label="收起目录"><ChevronDown size={15} /></button></div><div className="doc-project-label"><div className="mini-mark">{project.name.slice(0, 1)}</div><div><strong>{project.name}</strong><small>文档 v0.8.2</small></div></div><nav className="doc-tree"><button className="tree-item active"><FileText size={15} /> 快速开始</button><button className="tree-item"><ChevronRight size={14} /> 架构设计</button><button className="tree-item indent"><FileText size={14} /> Agent Runtime</button><button className="tree-item indent"><FileText size={14} /> Tool Protocol</button><button className="tree-item"><ChevronRight size={14} /> 开发指南</button><button className="tree-item"><ChevronRight size={14} /> API Reference</button></nav><div className="sidebar-bottom"><span className="meta-label">DOCUMENT STATUS</span><p><span className="status-dot" /> 已审核 · 公开可读</p></div></aside>
      <article className="document-article" onMouseUp={onSelection}>
        <div className="article-toolbar"><span className="meta-label">QUICK START / 01</span><div><button className="tool-button" title="复制标题链接"><Copy size={14} /> 链接</button><button className="tool-button" title="下载 Markdown"><Download size={14} /> 下载</button></div></div>
        <div className="article-content">
          <h1>快速开始</h1><p className="article-lead">Atlas Agent 是一个面向复杂任务的多 Agent 协作运行时。它把任务拆成可观察、可组合的步骤，让每次执行都能被理解和复盘。</p>
          <div className="callout"><Sparkles size={17} /><div><strong>阅读提示</strong><p>试着选中正文中的一段文字，然后点击浮出的评论按钮。</p></div></div>
          <h2>从一个清晰的任务开始</h2><p>一个好的 Agent 系统，首先要让目标被准确表达。Atlas 会先接收自然语言任务，再由规划器拆出检索、工具调用和结果检查等步骤。</p>
          <div className="code-block"><div className="code-head"><span>python</span><button className="code-copy" title="复制代码"><Copy size={14} /></button></div><pre><code><span className="code-purple">from</span> atlas <span className="code-purple">import</span> Runtime{`\n\n`}runtime = Runtime({`\n`}  model=<span className="code-green">"gpt-4.1"</span>,{`\n`}  max_steps=<span className="code-orange">8</span>,{`\n`}  trace=<span className="code-blue">True</span>,{`\n`}){`\n\n`}result = runtime.run(<span className="code-green">"分析这份用户反馈并给出行动计划"</span>)</code></pre></div>
          <h2>节点之间如何协作</h2><p>每个节点都需要声明输入、输出和失败策略。规划器负责安排顺序，执行器负责调用工具，评审器会在输出前检查引用和结果质量。</p>
          <div className="workflow-preview"><div className="workflow-node"><span>01</span><strong>规划器</strong><small>拆解目标</small></div><div className="workflow-arrow">→</div><div className="workflow-node active"><span>02</span><strong>执行器</strong><small>调用工具</small></div><div className="workflow-arrow">→</div><div className="workflow-node"><span>03</span><strong>评审器</strong><small>检查结果</small></div></div>
          <h2>安装依赖</h2><p>建议使用 Python 3.11 及以上版本。创建虚拟环境后，通过以下命令安装运行时：</p><div className="inline-command">$ pip install atlas-agent <button title="复制命令"><Copy size={14} /></button></div>
        </div>
        {commentComposerOpen && <div className="selection-composer"><div className="composer-quote">“{selectedQuote}”</div><textarea autoFocus value={draftComment} onChange={(event) => setDraftComment(event.target.value)} placeholder="写下你的评论..." /><div className="composer-actions"><button className="text-button" onClick={() => setCommentComposerOpen(false)}>取消</button><button className="primary-button small" onClick={onSubmitComment}><Send size={14} /> 发布评论</button></div></div>}
      </article>
      <aside className="comments-sidebar"><div className="comments-heading"><div><span className="meta-label">DISCUSSION</span><h3>文档评论 <span>{comments.length}</span></h3></div><button className="icon-button quiet" title="评论筛选" aria-label="评论筛选"><MoreHorizontal size={17} /></button></div><button className="new-comment-button" onClick={() => setCommentComposerOpen(true)}><MessageSquare size={15} /> 添加评论</button><div className="comment-list">{comments.map((comment) => <CommentCard key={comment.id} comment={comment} onResolve={() => onResolveComment(comment.id)} />)}</div><div className="realtime-note"><span className="status-dot" /> 评论实时同步中</div></aside>
    </section>
  )
}

function CommentCard({ comment, onResolve }: { comment: CommentItem; onResolve: () => void }) {
  return <article className={`comment-card ${comment.status === 'resolved' ? 'resolved' : ''}`}><div className="comment-card-head"><span className="avatar small-avatar">{comment.initials}</span><div><strong>{comment.user}</strong><small>{comment.time}</small></div>{comment.status === 'resolved' ? <Check size={15} className="resolved-icon" /> : <button className="comment-more" title="更多评论操作" aria-label="更多评论操作"><MoreHorizontal size={15} /></button>}</div><button className="comment-quote">“{comment.quote}”</button><p>{comment.text}</p>{comment.status === 'open' ? <div className="comment-actions"><button onClick={onResolve}>解决</button><button>回复</button></div> : <span className="resolved-label">已解决</span>}</article>
}

function OverviewView({ project, onRead }: { project: Project; onRead: () => void }) {
  return <section className="overview-view"><div className="overview-main"><span className="section-kicker">ABOUT THIS PROJECT</span><h2>{project.summary}</h2><p>{project.description} Atlas Agent 的设计目标是让复杂任务既能自动化，也能被开发者逐步理解。</p><div className="feature-list"><div><Check size={16} /><span>可回放的任务链路</span></div><div><Check size={16} /><span>工具调用和输出可审计</span></div><div><Check size={16} /><span>支持自定义 Agent 节点</span></div></div><button className="primary-button" onClick={onRead}>打开文档 <BookOpen size={16} /></button></div><div className={`overview-visual ${project.accent}`}><div className="visual-topline"><span>RUNTIME / 0.8.2</span><span>READY</span></div><div className="overview-lines"><span>planner</span><span>retriever</span><span>executor</span><span>reviewer</span></div><div className="overview-bottom"><span>4 个核心节点</span><span>trace enabled</span></div></div></section>
}

function CodeView({ project, onCopy }: { project: Project; onCopy: () => void }) {
  return <section className="code-view"><div className="file-tree"><div className="sidebar-heading"><span>代码目录</span><span className="meta-label">main</span></div>{codeFiles.map((file) => <button key={file.name} className={`file-item ${file.name === 'runtime.py' ? 'active' : ''}`}><span>{file.type === 'folder' ? <ChevronRight size={14} /> : <FileCode2 size={15} />}</span><span>{file.name}</span><small>{file.size}</small></button>)}<div className="file-tree-note"><GitBranch size={16} /><span>{project.repo}</span></div></div><div className="source-panel"><div className="source-head"><span>atlas / runtime.py</span><button className="tool-button" onClick={onCopy}><Copy size={14} /> 复制代码</button></div><pre className="source-code"><code><span className="line-number">01</span> <span className="code-purple">class</span> <span className="code-blue">Runtime</span>:{`\n`}<span className="line-number">02</span>     <span className="code-purple">def</span> <span className="code-blue">__init__</span>(self, model, max_steps=<span className="code-orange">8</span>):{`\n`}<span className="line-number">03</span>         self.model = model{`\n`}<span className="line-number">04</span>         self.max_steps = max_steps{`\n`}<span className="line-number">05</span>         self.trace = TraceStore(){`\n\n`}<span className="line-number">07</span>     <span className="code-purple">def</span> <span className="code-blue">run</span>(self, task):{`\n`}<span className="line-number">08</span>         plan = self.planner.create(task){`\n`}<span className="line-number">09</span>         <span className="code-purple">for</span> step <span className="code-purple">in</span> plan.steps:{`\n`}<span className="line-number">10</span>             result = self.execute(step){`\n`}<span className="line-number">11</span>             self.trace.append(result){`\n`}<span className="line-number">12</span>         <span className="code-purple">return</span> self.reviewer.check(self.trace)</code></pre></div></section>
}

function DownloadView({ onDownload }: { onDownload: () => void }) {
  return <section className="download-view"><div className="download-intro"><span className="section-kicker">RELEASES / 04</span><h2>选择一个资源开始。</h2><p>当前展示的是项目公开资源。下载记录会计入项目统计，资源由作者维护。</p><button className="outline-button" onClick={onDownload}><Download size={15} /> 下载代码包</button></div><div className="resource-list"><div className="resource-row"><div className="resource-icon"><Code2 size={18} /></div><div><strong>atlas-agent-v0.8.2.tar.gz</strong><small>代码包 · 2.8 MB · MIT</small></div><span>v0.8.2</span><button className="icon-button" title="下载代码包" aria-label="下载代码包" onClick={onDownload}><Download size={16} /></button></div><div className="resource-row"><div className="resource-icon"><FileText size={18} /></div><div><strong>项目文档.pdf</strong><small>文档 · 1.2 MB · 更新于昨天</small></div><span>PDF</span><button className="icon-button" title="下载文档" aria-label="下载文档" onClick={onDownload}><Download size={16} /></button></div><div className="resource-row"><div className="resource-icon"><Play size={18} /></div><div><strong>产品演示</strong><small>Bilibili 外链 · 06:42</small></div><span>VIDEO</span><button className="icon-button" title="打开演示视频" aria-label="打开演示视频" onClick={onDownload}><ArrowUpRight size={16} /></button></div></div></section>
}

function LoginModal({ onClose, onLogin }: { onClose: () => void; onLogin: () => void }) {
  return <div className="modal-backdrop" onMouseDown={onClose}><div className="login-modal" onMouseDown={(event) => event.stopPropagation()}><button className="modal-close icon-button" title="关闭登录窗口" aria-label="关闭登录窗口" onClick={onClose}><X size={18} /></button><span className="brand-mark large">新</span><h2>登录新猿译码</h2><p>登录后可以收藏项目、参与讨论和关注更新。</p><button className="provider-button github" onClick={onLogin}><GitBranch size={17} /> 使用 GitHub 登录 <ArrowUpRight size={14} /></button><button className="provider-button wechat" onClick={onLogin}><span className="wechat-icon">微</span> 使用微信登录 <ArrowUpRight size={14} /></button><div className="login-divider"><span>或</span></div><button className="email-login" onClick={onLogin}>使用邮箱继续</button><small>继续即表示你同意社区使用规范。</small></div></div>
}

export default App
