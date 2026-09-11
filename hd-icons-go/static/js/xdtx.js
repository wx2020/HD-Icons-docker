// By 小迪同学，mail:d_songbo@126.com
// 哔哩哔哩：https://space.bilibili.com/32313260
// YouTube：https://www.youtube.com/@xiaoditx
// Github：https://github.com/xushier

        let currentImageType = 'all';
        let currentSearchQuery = '';
        let selectedFiles = [];

        // 类别映射
        const categoryMap = {
            'all': '所有',
            'border-radius': '圆角',
            'circle': '圆形',
            'svg': '矢量',
            'upload': '自定'
        };

        // 设置图片类型
        function setImageType(type) {
            currentImageType = type;
            currentSearchQuery = '';
            document.getElementById('search').value = '';
            updateButtonState(type);
            loadImages();
        }

        // 更新按钮状态
        function updateButtonState(selectedType) {
            const buttons = ['all', 'border-radius', 'circle', 'svg', 'upload'];
            buttons.forEach(button => {
                const btnElement = document.getElementById(`btn-${button}`);
                if (button === selectedType) {
                    btnElement.classList.add('bg-fuchsia-600');
                } else {
                    btnElement.classList.remove('bg-fuchsia-600');
                }
            });
        }

        // 复制地址前缀：后端 CUSTOM_URL 注入（云端模式），为空则同源
        function copyBase() {
            if (window.CUSTOM_URL && window.CUSTOM_URL.trim() !== '') {
                return window.CUSTOM_URL.replace(/\/+$/, '');
            }
            return window.location.origin;
        }

        // 加载图片
        function loadImages() {
            const search = document.getElementById('search').value;
            currentSearchQuery = search;
            fetch(`/images?type=${currentImageType}&search=${search}`)
                .then(response => response.json())
                .then(images => {
                    const container = document.getElementById('image-container');
                    container.innerHTML = '';
                    if (images.length === 0) {
                        updateStatus(0); // 如果没有搜索结果，显示“未找到”
                        return;
                    }
                    images.forEach(img => {
                        const imgElement = document.createElement('img');
                        // SVG 文件直接加载原图，其他文件加载缩略图
                        const isSvg = img.name.toLowerCase().endsWith('.svg');
                        const imageUrl = isSvg
                            ? `${window.location.origin}/icons/${img.type === 'upload' ? 'upload' : 'HD-Icons/' + img.type}/${img.name}`
                            : `${window.location.origin}/icons/thumbnails/${img.type}/${img.name}`;
                        imgElement.src = imageUrl;
                        imgElement.dataset.original = `${copyBase()}/icons/${img.type === 'upload' ? 'upload' : 'HD-Icons/' + img.type}/${img.name}`; // 保存原图路径（复制用，支持 CUSTOM_URL）
                        imgElement.classList.add('w-16', 'h-16', 'object-cover', 'cursor-pointer', 'lazy');
        
                        const filenameDiv = document.createElement('div');
                        filenameDiv.textContent = img.name;
                        filenameDiv.classList.add('filename', 'text-center', 'mt-2', 'px-2', 'py-1');
        
                        const imageWrapper = document.createElement('div');
                        imageWrapper.classList.add('image-card', 'flex', 'flex-col', 'items-center', 'p-2', 'rounded-lg', 'relative', 'fade-in');
                        imageWrapper.appendChild(imgElement);
                        imageWrapper.appendChild(filenameDiv);
        
                        // 添加放大和删除按钮
                        const imageButtons = document.createElement('div');
                        imageButtons.classList.add('image-buttons', 'flex', 'justify-center', 'w-full', 'mt-2');
                        const zoomButton = document.createElement('button');
                        zoomButton.innerHTML = '<i class="fas fa-search-plus text-purple-600 hover:text-white"></i>';
                        zoomButton.classList.add('p-2', 'hover:bg-purple-600', 'rounded-lg', 'transition-colors', 'duration-300', 'focus:outline-none');
                        zoomButton.onclick = function(event) {
                            event.stopPropagation();
                            showPreviewPopup(imgElement.dataset.original);
                        };
        
                        // 如果是自定义分类，添加删除按钮
                        if (img.type === 'upload') {
                            const deleteButton = document.createElement('button');
                            deleteButton.innerHTML = '<i class="fas fa-trash text-red-500 hover:text-white"></i>';
                            deleteButton.classList.add('p-2', 'hover:bg-red-500', 'rounded-lg', 'transition-colors', 'duration-300', 'focus:outline-none', 'ml-2');
                            deleteButton.onclick = function(event) {
                                event.stopPropagation();
                                showDeleteConfirmation(img.name, imageWrapper);
                            };
                            imageButtons.appendChild(deleteButton);
                        }
        
                        imageButtons.appendChild(zoomButton);
                        imageWrapper.appendChild(imageButtons);
        
                        // 复制成功提示
                        const copySuccess = document.createElement('div');
                        copySuccess.classList.add('copy-success');
                        copySuccess.textContent = '复制成功';
                        copySuccess.style.borderRadius = '1.5rem'; // 保留圆角
                        imageWrapper.appendChild(copySuccess);
        
                        // 初始化 clipboard.js
                        new ClipboardJS(imageWrapper, {
                            text: function () {
                                return imgElement.dataset.original; // 复制原图地址
                            }
                        }).on('success', function () {
                            copySuccess.classList.add('active');
                            setTimeout(() => {
                                copySuccess.classList.remove('active');
                            }, 2000);
                        });
        
                        container.appendChild(imageWrapper);
                    });
        
                    // 更新状态显示
                    updateStatus(images.length);
        
                    // 初始化懒加载
                    initLazyLoad();
        
                    // 隐藏加载动画
                    document.getElementById('loading-spinner').style.display = 'none';
                })
                // .catch(error => {
                //     console.error('加载图片时出错:', error);
                //     document.getElementById('status').innerHTML = `
                //         <div class="status-button">加载图片失败，请检查网络连接或刷新页面</div>
                //     `;
                // });
        }

        // 初始化懒加载
        function initLazyLoad() {
            const lazyImages = document.querySelectorAll('img.lazy');
            const observer = new IntersectionObserver((entries, observer) => {
                entries.forEach(entry => {
                    if (entry.isIntersecting) {
                        const img = entry.target;
                        img.src = img.dataset.original; // 加载原图
                        img.classList.remove('lazy');
                        observer.unobserve(img);
                    }
                });
            });

            lazyImages.forEach(img => {
                observer.observe(img);
            });
        }

        // 更新状态显示
        function updateStatus(count) {
            const statusElement = document.getElementById('status');
            if (currentSearchQuery) {
                if (count > 0) {
                    statusElement.innerHTML = `
                        <div class="status-button">共搜索到 ${count} 个</div>
                    `;
                } else {
                    statusElement.innerHTML = `
                        <div class="status-button">未找到，试试更短的关键词？</div>
                        <a href="https://github.com/xushier/HD-Icons/issues/new/choose" target="_blank" class="status-button feedback-button">向作者反馈</a>
                    `;
                }
            } else {
                statusElement.innerHTML = `
                    <div class="status-button">当前类别：${categoryMap[currentImageType]}</div>
                    <div class="status-button">共有 ${count} 个</div>
                `;
            }
        }

        // 处理回车键
        function handleKeyPress(event) {
            if (event.key === 'Enter') {
                loadImages();
            }
        }

        // 切换日间/夜间模式
        function toggleMode() {
            const body = document.body;
            const toggleButton = document.getElementById('toggle-mode');
            if (body.classList.contains('day-mode')) {
                body.classList.remove('day-mode');
                body.classList.add('night-mode');
                toggleButton.innerHTML = '<i class="fas fa-sun"></i><span class="hidden md:inline">日间模式</span>';
            } else {
                body.classList.remove('night-mode');
                body.classList.add('day-mode');
                toggleButton.innerHTML = '<i class="fas fa-moon"></i><span class="hidden md:inline">夜间模式</span>';
            }
        }

        // 检测系统主题偏好并自动切换模式
        function autoToggleMode() {
            const isDarkMode = window.matchMedia('(prefers-color-scheme: dark)').matches;
            const body = document.body;
            const toggleButton = document.getElementById('toggle-mode');

            if (isDarkMode) {
                body.classList.remove('day-mode');
                body.classList.add('night-mode');
                toggleButton.innerHTML = '<i class="fas fa-sun"></i><span class="hidden md:inline">日间模式</span>';
            } else {
                body.classList.remove('night-mode');
                body.classList.add('day-mode');
                toggleButton.innerHTML = '<i class="fas fa-moon"></i><span class="hidden md:inline">夜间模式</span>';
            }
        }

        // 检查更新
        function checkForUpdates() {
            fetch('/check-update')
                .then(response => response.json())
                .then(data => {
                    alert(data.status);
                });
        }

        // 定期自动检查更新
        function autoCheckUpdates() {
            setInterval(() => {
                checkForUpdates();
            }, 3600000); // 每小时检查一次
        }

        function showUploadPopup() {
            const popup = document.getElementById('upload-popup');
            const overlay = document.createElement('div');
            overlay.classList.add('overlay', 'active');
            overlay.style.zIndex = '1001'; // 设置 overlay 的 z-index
            popup.style.zIndex = '1002'; // 设置上传弹窗的 z-index
            document.body.appendChild(overlay);
            popup.classList.add('active');
        
            // 点击周围区域关闭弹窗
            overlay.addEventListener('click', () => {
                closeUploadPopup();
            });
        }

        // 关闭上传弹窗
        function closeUploadPopup() {
            const popup = document.getElementById('upload-popup');
            const overlay = document.querySelector('.overlay.active');
            if (overlay) {
                document.body.removeChild(overlay);
            }
            popup.classList.remove('active');
        }

        // 允许的文件格式
        const allowedFormats = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp', 'tiff', 'apng', 'ico', 'tif'];
        
        // 处理文件选择
        function handleFileSelect() {
            const files = document.getElementById('file-input').files;
            if (files.length > 0) {
                selectedFiles = Array.from(files).filter(file => {
                    const extension = file.name.split('.').pop().toLowerCase();
                    return allowedFormats.includes(extension);
                });
        
                if (selectedFiles.length < files.length) {
                    alert('文件格式不支持！仅支持：png, jpg, jpeg, gif, webp, svg, bmp, tiff, apng, ico, tif');
                }
        
                updateFileList();
            }
        }

        // 更新文件列表
        function updateFileList() {
            const fileList = document.getElementById('file-list');
            selectedFiles.forEach((file, index) => {
                const fileItem = document.createElement('div');
                fileItem.textContent = `${index + 1}. ${file.name}`;
                fileItem.classList.add('text-sm', 'text-gray-700');
                fileList.appendChild(fileItem);
            });
        }

        // 上传文件
        function uploadFiles() {
            if (selectedFiles.length === 0) {
                alert('请先选择文件！');
                return;
            }
        
            const formData = new FormData();
            selectedFiles.forEach(file => {
                formData.append('file', file); // 添加所有文件
            });
        
            const xhr = new XMLHttpRequest();
            xhr.open('POST', '/upload-image', true);
        
            xhr.upload.onprogress = function(event) {
                if (event.lengthComputable) {
                    const percentComplete = (event.loaded / event.total) * 100;
                    document.getElementById('progress-bar').style.width = percentComplete + '%';
                    document.getElementById('upload-status').textContent = '上传中...';
                }
            };
        
            xhr.onload = function() {
                if (xhr.status === 200) {
                    document.getElementById('upload-status').textContent = '上传完成';
                    selectedFiles = []; // 清空已选择文件
                    updateFileList(); // 更新文件列表
                    loadImages(); // 刷新图片显示
                } else {
                    document.getElementById('upload-status').textContent = '上传失败';
                }
            };
        
            xhr.send(formData);
        }

        // 显示预览弹窗
        function showPreviewPopup(imageUrl) {
            const popup = document.createElement('div');
            popup.classList.add('popup');
            const popupImg = document.createElement('img');
            popupImg.src = imageUrl;
            popupImg.style.maxWidth = '90%';
            popupImg.style.maxHeight = '90%';
            popupImg.style.margin = '0 auto'; // 居中
            popupImg.style.objectFit = 'contain'; // 保持图片比例
            popupImg.style.opacity = '0'; // 初始透明度为 0
            popupImg.style.transform = 'scale(0.8)'; // 初始缩放为 0.8
            popup.appendChild(popupImg);
        
            const overlay = document.createElement('div');
            overlay.classList.add('overlay');
        
            document.body.appendChild(overlay);
            document.body.appendChild(popup);
        
            // 添加动画
            setTimeout(() => {
                popup.classList.add('active');
                overlay.classList.add('active');
                popupImg.style.opacity = '1'; // 透明度变为 1
                popupImg.style.transform = 'scale(1)'; // 缩放变为 1
                popupImg.style.transition = 'opacity 0.3s ease, transform 0.3s ease'; // 添加过渡效果
            }, 10);
        
            // 点击任意位置关闭弹窗
            overlay.addEventListener('click', () => {
                popup.classList.remove('active');
                overlay.classList.remove('active');
                setTimeout(() => {
                    document.body.removeChild(overlay);
                    document.body.removeChild(popup);
                }, 300);
            });
        }

        // 显示删除确认弹窗
        function showDeleteConfirmation(imageName, imageWrapper) {
            const confirmationPopup = document.createElement('div');
            confirmationPopup.classList.add('copy-success', 'active');
            confirmationPopup.style.width = imageWrapper.offsetWidth + 'px';
            confirmationPopup.style.height = imageWrapper.offsetHeight + 'px';
            confirmationPopup.style.display = 'flex';
            confirmationPopup.style.flexDirection = 'column';
            confirmationPopup.style.justifyContent = 'center';
            confirmationPopup.style.alignItems = 'center';
            confirmationPopup.style.fontSize = '1rem'; // 增大文字大小
            confirmationPopup.style.borderRadius = '0'; // 取消圆角
            confirmationPopup.innerHTML = `
                <p style="margin-bottom: 0.5rem;">确认删除？</p> <!-- 减少下边距 -->
                <div style="display: flex; gap: 0.5rem; flex-direction: column; align-items: center;">
                    <button onclick="event.stopPropagation(); deleteImage('${imageName}', this.parentElement.parentElement)" class="p-1 bg-green-500 text-white rounded-lg hover:bg-green-600 focus:outline-none focus:ring-2 focus:ring-green-500">确认</button>
                    <button onclick="event.stopPropagation(); closeDeletePopup(this.parentElement.parentElement)" class="p-1 bg-red-500 text-white rounded-lg hover:bg-red-600 focus:outline-none focus:ring-2 focus:ring-red-500">取消</button>
                </div>
            `;
            imageWrapper.appendChild(confirmationPopup);
        }

        // 删除图片
        function deleteImage(imageName, popup) {
            fetch('/delete-image', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ image_name: imageName }),
            })
            .then(response => response.json())
            .then(data => {
                if (data.status === 'success') {
                    popup.remove();
                    loadImages();
                }
            });
        }

        // 关闭删除确认弹窗
        function closeDeletePopup(popup) {
            popup.remove();
        }

        // 初始化页面
        document.addEventListener('DOMContentLoaded', function () {
            // 自动切换模式
            autoToggleMode();

            // 监听系统主题变化
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
                autoToggleMode();
            });

            // 默认加载所有图片
            loadImages();

            // 绑定模式切换按钮
            document.getElementById('toggle-mode').addEventListener('click', toggleMode);

            // 绑定检查更新按钮
            document.getElementById('check-update').addEventListener('click', checkForUpdates);

            // 初始化按钮状态
            updateButtonState(currentImageType);
            
            // 启动自动检查更新
            autoCheckUpdates();

            // 绑定上传弹窗事件
            document.getElementById('drop-zone').addEventListener('click', function() {
                document.getElementById('file-input').click();
            });

            document.getElementById('drop-zone').addEventListener('dragover', function(event) {
                event.preventDefault();
                event.stopPropagation();
                event.dataTransfer.dropEffect = 'copy';
            });

            document.getElementById('drop-zone').addEventListener('drop', function(event) {
                event.preventDefault();
                event.stopPropagation();
                const files = event.dataTransfer.files;
                if (files.length > 0) {
                    selectedFiles = Array.from(files); // 更新文件列表
                    updateFileList();
                }
            });
        });