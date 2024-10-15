# Hệ thống Xử lý Output

## Giới thiệu

Hệ thống này được thiết kế để xử lý và gửi các output một cách hiệu quả trong một ứng dụng phân tán. Nó bao gồm ba thành phần chính: Dispatcher, Worker, và WorkerPool. Hệ thống này sử dụng các kênh (channels) và goroutines của Go để đạt được xử lý đồng thời và hiệu suất cao.

## Các Thành phần Chính

### 1. Dispatcher

Dispatcher là thành phần chịu trách nhiệm nhận và tích lũy các output riêng lẻ, sau đó gửi chúng theo lô đến WorkerPool để xử lý.

#### Cấu trúc chính:
- `inputChannel`: Kênh nhận các output riêng lẻ.
- `outputChannel`: Kênh gửi các lô output đã được tích lũy.
- `buffer`: Slice để tích lũy các output.
- `maxSize`: Kích thước tối đa của buffer trước khi flush.
- `flushInterval`: Khoảng thời gian để force flush buffer.

#### Cách hoạt động:
- Dispatcher chạy trong một goroutine riêng.
- Nó liên tục lắng nghe các output mới từ `inputChannel`.
- Các output được thêm vào buffer cho đến khi đạt `maxSize` hoặc `flushInterval` trôi qua.
- Khi buffer đầy hoặc hết thời gian, Dispatcher sẽ flush buffer bằng cách gửi nó qua `outputChannel`.

### 2. Worker

Worker là đơn vị xử lý công việc cơ bản. Mỗi Worker có thể xử lý một lô output.

#### Cấu trúc chính:
- `id`: ID duy nhất của Worker.
- `jobChannel`: Kênh nhận các công việc (lô output) cần xử lý.
- `runningMode`: Chế độ chạy (server hoặc agent).

#### Cách hoạt động:
- Worker chạy trong một goroutine riêng.
- Nó liên tục đăng ký sẵn sàng nhận công việc với WorkerPool.
- Khi nhận được công việc, Worker sẽ xử lý lô output dựa trên `runningMode`.
- Trong chế độ server, nó gửi output đến một endpoint từ xa.
- Trong chế độ agent, nó cập nhật kết quả công việc cục bộ.

### 3. WorkerPool

WorkerPool quản lý một nhóm các Worker và phân phối công việc giữa chúng.

#### Cấu trúc chính:
- `workers`: Slice chứa tất cả các Worker.
- `jobChannel`: Kênh nhận các lô output từ Dispatcher.
- `workerChannel`: Kênh để các Worker đăng ký sẵn sàng nhận công việc.

#### Cách hoạt động:
- WorkerPool khởi tạo và quản lý một số lượng Worker được chỉ định.
- Nó lắng nghe các lô output từ `jobChannel`.
- Khi nhận được lô output, nó chọn một Worker đang rảnh và gửi công việc cho Worker đó.
- WorkerPool đảm bảo rằng công việc được phân phối đều giữa các Worker.

## Luồng Dữ liệu

1. Các output riêng lẻ được gửi đến Dispatcher thông qua `inputChannel`.
2. Dispatcher tích lũy các output trong buffer của nó.
3. Khi buffer đầy hoặc hết thời gian, Dispatcher flush buffer đến WorkerPool.
4. WorkerPool nhận lô output và gửi nó đến một Worker đang rảnh.
5. Worker xử lý lô output (gửi đến server hoặc cập nhật cục bộ, tùy thuộc vào chế độ).
6. Quá trình này tiếp tục cho đến khi hệ thống được dừng lại.

## Ưu điểm của Hệ thống

1. **Hiệu quả**: Xử lý theo lô giúp giảm overhead của network và I/O.
2. **Khả năng mở rộng**: Có thể dễ dàng thêm nhiều Worker để xử lý khối lượng công việc lớn hơn.
3. **Linh hoạt**: Hỗ trợ cả chế độ server và agent.
4. **Đồng thời**: Sử dụng goroutines và channels để xử lý đồng thời hiệu quả.
5. **Kiểm soát tốt**: Cơ chế flush theo kích thước và thời gian giúp cân bằng giữa độ trễ và thông lượng.

Hệ thống này cung cấp một giải pháp mạnh mẽ và linh hoạt để xử lý output trong các ứng dụng Go, đặc biệt phù hợp cho các hệ thống phân tán hoặc các ứng dụng cần xử lý một lượng lớn dữ liệu output.
