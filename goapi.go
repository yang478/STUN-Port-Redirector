package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// JSON 文件路径
const (
	jsonFilePath        = "/app/data.json"
	redirectMappingPath = "/app/redirect_mapping.json"
	debounceTime        = 100 * time.Millisecond // 去抖动时间
)

// 从环境变量中读取 Bearer Token
var validBearerToken = os.Getenv("BEARER_TOKEN")

// 内存缓存
var (
	dataCache        map[string]interface{}
	redirectMapping  map[string]string
	dataCacheLock    sync.RWMutex
	redirectLock     sync.RWMutex
	lastDataHash     string
	lastRedirectHash string
	debounceTimer    *time.Timer
)

// 初始化缓存
func init() {
	// 检查环境变量是否设置
	if validBearerToken == "" {
		log.Fatalf("BEARER_TOKEN environment variable is not set")
	}

	dataCache = make(map[string]interface{})
	redirectMapping = make(map[string]string)

	// 初始加载数据
	loadDataToCache()
	loadRedirectMapping()

	// 启动定时任务，定期将缓存写入文件
	go func() {
		for {
			time.Sleep(30 * time.Second) // 每 30 秒同步一次
			saveCacheToFile()
		}
	}()

	// 启动文件监听
	go watchFiles(redirectMappingPath, loadRedirectMapping)
	go watchFiles(jsonFilePath, loadDataToCache)
}

// 计算文件内容的 MD5 哈希值
func getFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// 从文件加载数据到缓存
func loadDataToCache() {
	// 计算当前文件内容的哈希值
	currentHash, err := getFileHash(jsonFilePath)
	if err != nil {
		log.Printf("Failed to calculate file hash: %v", err)
		return
	}

	// 如果哈希值没有变化，则跳过重新加载
	if currentHash == lastDataHash {
		return
	}

	// 更新哈希值
	lastDataHash = currentHash

	file, err := os.Open(jsonFilePath)
	if err != nil {
		log.Printf("Failed to open JSON file: %v", err)
		return
	}
	defer file.Close()

	// 检查文件大小，避免解码空文件
	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Failed to stat JSON file: %v", err)
		return
	}
	if fileInfo.Size() == 0 {
		log.Println("JSON file is empty, skipping decode.")
		return
	}

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&dataCache); err != nil {
		log.Printf("Failed to decode JSON file: %v", err)
		return
	}

	log.Println("Data reloaded successfully.")
}

// 将缓存写入文件
func saveCacheToFile() {
	dataCacheLock.RLock()
	defer dataCacheLock.RUnlock()

	file, err := os.Create(jsonFilePath)
	if err != nil {
		log.Printf("Failed to create JSON file: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(dataCache); err != nil {
		log.Printf("Failed to encode JSON data: %v", err)
	}
}

// 加载重定向映射表
func loadRedirectMapping() {
	// 计算当前文件内容的哈希值
	currentHash, err := getFileHash(redirectMappingPath)
	if err != nil {
		log.Printf("Failed to calculate file hash: %v", err)
		return
	}

	// 如果哈希值没有变化，则跳过重新加载
	if currentHash == lastRedirectHash {
		return
	}

	// 更新哈希值
	lastRedirectHash = currentHash

	file, err := os.Open(redirectMappingPath)
	if err != nil {
		log.Printf("Failed to open redirect mapping file: %v", err)
		return
	}
	defer file.Close()

	// 检查文件大小，避免解码空文件
	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("Failed to stat redirect mapping file: %v", err)
		return
	}
	if fileInfo.Size() == 0 {
		log.Println("Redirect mapping file is empty, skipping decode.")
		return
	}

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&redirectMapping); err != nil {
		log.Printf("Failed to decode redirect mapping file: %v", err)
		return
	}

	log.Println("Redirect mapping reloaded successfully.")
}

// 监听文件变化
func watchFiles(filename string, reloadFunc func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	defer watcher.Close()

	err = watcher.Add(filename)
	if err != nil {
		log.Fatalf("Failed to add file to watcher: %v", err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Printf("File %s changed, scheduling reload...", filename)

				// 去抖动机制：等待 debounceTime 后再重新加载
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(debounceTime, reloadFunc)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)
		}
	}
}

// 认证中间件
func authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != validBearerToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// 保存数据接口
func saveDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体中的 JSON 数据
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var newData map[string]interface{}
	if err := json.Unmarshal(body, &newData); err != nil {
		http.Error(w, "Invalid JSON data", http.StatusBadRequest)
		return
	}

	// 更新缓存
	dataCacheLock.Lock()
	for k, v := range newData {
		dataCache[k] = v
	}
	dataCacheLock.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{"message": "Data saved successfully"}`)
}

// 获取数据接口
func getDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从缓存中读取数据
	dataCacheLock.RLock()
	defer dataCacheLock.RUnlock()

	// 返回 JSON 数据
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dataCache)
}

// 重定向处理
func redirectHandler(w http.ResponseWriter, r *http.Request) {
	redirectLock.RLock()
	defer redirectLock.RUnlock()

	log.Printf("INFO: Received request: Host=%s, URL=%s, RemoteAddr=%s", r.Host, r.URL.String(), r.RemoteAddr)

	// 提取请求的端口号
	host := r.Host
	var port string
	var err error

	// 检查 Host 是否包含端口号
	if strings.Contains(host, ":") {
		_, port, err = net.SplitHostPort(host)
		if err != nil {
			log.Printf("ERROR: Failed to extract port from host: %v", err)
			http.Error(w, "Invalid Host", http.StatusBadRequest)
			return
		}
	} else {
		// 如果 Host 不包含端口号，直接使用默认端口
		dataCacheLock.RLock()
		portValue, ok := dataCache["port"]
		dataCacheLock.RUnlock()

		if !ok {
			log.Printf("ERROR: 'port' key not found in dataCache")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 检查 portValue 是否为 float64 类型
		portFloat, ok := portValue.(float64)
		if !ok {
			log.Printf("ERROR: 'port' value is not a float64: %v", portValue)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		port = fmt.Sprintf("%d", int(portFloat))
	}

	// 构建重定向规则的键
	key := fmt.Sprintf("*:%s", port)

	log.Printf("INFO: Looking up redirect rule for key: %s", key) // 打印查找的键

	// 从内存中的 redirectMapping 查找重定向规则
	if newURL, ok := redirectMapping[key]; ok {
		// 如果 URL 不包含协议，默认添加 http://
		if !strings.HasPrefix(newURL, "http") {
			newURL = fmt.Sprintf("http://%s", newURL)
		}

		// 直接将 data.json 中的端口号附加到目标 URL 后面
		dataCacheLock.RLock()
		portValue, ok := dataCache["port"]
		dataCacheLock.RUnlock()

		if !ok {
			log.Printf("ERROR: 'port' key not found in dataCache")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		portFloat, ok := portValue.(float64)
		if !ok {
			log.Printf("ERROR: 'port' value is not a float64: %v", portValue)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		newURL = fmt.Sprintf("%s:%d", strings.TrimRight(newURL, "/"), int(portFloat))

		log.Printf("INFO: Redirecting to: %s", newURL)
		http.Redirect(w, r, newURL, http.StatusFound)
	} else {
		log.Printf("WARN: No redirect rule found for %s", key)
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

// 启动 HTTP 服务器
func startHTTPServers(ports []string) {
	for _, port := range ports {
		go func(p string) {
			log.Printf("Starting HTTP server on port %s...", p)

			// 检查端口是否被占用
			ln, err := net.Listen("tcp", ":"+p)
			if err != nil {
				log.Fatalf("Failed to listen on port %s: %v", p, err)
			}
			defer ln.Close()

			server := &http.Server{
				Addr: ":" + p,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					redirectHandler(w, r)
				}),
			}

			go func() {
				if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
					log.Fatalf("Failed to start HTTP server on port %s: %v", p, err)
				}
			}()

			log.Printf("HTTP server started on port %s", p)

			// 等待程序退出信号
			<-make(chan struct{})
		}(port)
	}
}

func main() {
	// 设置 API 路由
	http.HandleFunc("/api/save", authenticate(saveDataHandler))
	http.HandleFunc("/api/get", authenticate(getDataHandler))

	// 启动 API 服务器，监听 5000 端口
	go func() {
		log.Printf("Starting API server on port 5000...")
		if err := http.ListenAndServe(":5000", nil); err != nil {
			log.Fatalf("Failed to start API server on port 5000: %v", err)
		}
	}()

	// 从 redirect_mapping.json 中提取需要监听的端口
	var ports []string
	redirectLock.RLock()
	for key := range redirectMapping {
		_, port, err := net.SplitHostPort(key)
		if err != nil {
			log.Printf("WARN: Invalid key in redirect_mapping.json: %s", key)
			continue
		}
		ports = append(ports, port)
	}
	redirectLock.RUnlock()

	// 启动 HTTP 服务器
	startHTTPServers(ports)

	// 保持主程序运行
	select {}
}
