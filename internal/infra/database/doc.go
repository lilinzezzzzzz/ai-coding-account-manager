// Package database 负责面向本地单实例部署的 SQLite、GORM、migration 和事务基础设施。
//
// 当前实现不支持多个应用实例共享同一个 SQLite 数据库文件，也不支持将数据库文件放在
// 网络文件系统上。多实例部署需要共享数据库以及 operation ownership、lease、heartbeat
// 和任务协调机制。
package database
