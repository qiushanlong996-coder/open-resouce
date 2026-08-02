package main

import "sync"

// best-effort 后台任务的登记处。
//
// 项目里有若干「失败只记日志、不阻塞主动作」的异步写入：加经验、写搜索索引、
// 累计项目指标。它们都读包级依赖（authRepositoryStore、searchIndexStore 等）。
//
// 生产环境这些依赖只在启动时赋值一次，之后只读，所以没有竞态。
// 但测试会替换它们、并在 t.Cleanup 里换回去——这时如果还有在途的后台任务正在
// 读同一个变量，就是真实的数据竞争（go test -race 会直接判定测试失败）。
//
// 因此这里给所有 best-effort 任务挂一个 WaitGroup：
//   - 测试在替换/恢复包级依赖前调用 waitForBackgroundTasks 排空在途任务；
//   - 优雅退出时同样排空，避免丢掉已经收下的经验与索引写入。
var backgroundTasks sync.WaitGroup

// runBestEffort 起一个受跟踪的后台任务。
// 语义与直接 `go func(){}()` 相同，只是可被等待。
func runBestEffort(task func()) {
	backgroundTasks.Add(1)
	go func() {
		defer backgroundTasks.Done()
		task()
	}()
}

// waitForBackgroundTasks 等待所有在途的 best-effort 任务结束。
func waitForBackgroundTasks() {
	backgroundTasks.Wait()
}
