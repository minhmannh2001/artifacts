# Mô tả Sơ đồ Luồng và Sơ đồ Tuần tự

## Sơ đồ Luồng: Output Dispatcher

Sơ đồ luồng mô tả quá trình hoạt động của Output Dispatcher, một thành phần quan trọng trong hệ thống xử lý output.

### Các bước chính:

1. **Khởi tạo Dispatcher**: Quá trình bắt đầu với việc khởi tạo Dispatcher.

2. **Bắt đầu Goroutine**: Một goroutine mới được khởi động để xử lý các sự kiện.

3. **Chờ Sự kiện**: Dispatcher liên tục chờ đợi ba loại sự kiện chính:
   - Dữ liệu đầu vào
   - Hết thời gian chờ
   - Tín hiệu dừng

4. **Xử lý Dữ liệu đầu vào**:
   - Khi nhận được dữ liệu, nó được thêm vào buffer.
   - Nếu buffer đầy, dữ liệu được flush (gửi đi) và timer được đặt lại.

5. **Xử lý Hết thời gian chờ**:
   - Khi timer hết hạn, kiểm tra xem buffer có trống không.
   - Nếu buffer không trống, thực hiện flush.
   - Đặt lại timer.

6. **Xử lý Tín hiệu dừng**:
   - Khi nhận được tín hiệu dừng, flush dữ liệu còn lại trong buffer.
   - Đóng kênh output và kết thúc quá trình.

Sơ đồ này giúp hiểu rõ cách Dispatcher quản lý việc tích lũy và gửi dữ liệu, cân bằng giữa hiệu suất (xử lý theo lô) và độ trễ (flush định kỳ).

## Sơ đồ Tuần tự: Worker Pool và Dispatcher

Sơ đồ tuần tự mô tả tương tác giữa các thành phần chính trong hệ thống: Main (chương trình chính), Dispatcher, WorkerPool, và Worker.

### Các bước và tương tác chính:

1. **Khởi tạo**:
   - Main tạo Dispatcher và WorkerPool.
   - Main kích hoạt Dispatcher và WorkerPool.

2. **Khởi động Workers**:
   - WorkerPool khởi động từng Worker.

3. **Xử lý Output**:
   - Dispatcher liên tục nhận và buffer các output.
   - Khi buffer đầy hoặc hết thời gian chờ, Dispatcher gửi các output đã buffer đến WorkerPool.
   - WorkerPool gán công việc cho một Worker có sẵn.
   - Worker xử lý các output.

4. **Kết thúc**:
   - Main gửi tín hiệu dừng đến Dispatcher và WorkerPool.
   - Dispatcher flush dữ liệu còn lại.
   - WorkerPool dừng tất cả các Worker.

Sơ đồ này thể hiện rõ cách các thành phần tương tác với nhau, từ việc nhận dữ liệu ban đầu đến xử lý cuối cùng bởi các Worker. Nó cũng minh họa tính đồng thời của hệ thống, với nhiều Worker có thể xử lý công việc cùng một lúc.

Cả hai sơ đồ đều giúp hiểu rõ hơn về luồng dữ liệu và logic xử lý trong hệ thống, từ đó giúp người đọc nắm bắt được cách hệ thống hoạt động một cách trực quan và hiệu quả.
