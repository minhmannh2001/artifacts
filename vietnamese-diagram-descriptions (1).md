# Mô tả Sơ đồ Tuần tự và Sơ đồ Luồng

## 1. Sơ đồ Tuần tự: Tương tác giữa orenctl, orenctl worker và Zeebe

Sơ đồ tuần tự mô tả quá trình tương tác giữa ba thành phần chính: Zeebe, orenctl và orenctl worker.

### Các bước chính:

1. **Lấy nhiệm vụ**: orenctl gửi yêu cầu đến Zeebe để lấy các nhiệm vụ đã được kích hoạt, với cấu hình thời gian chờ.

2. **Xử lý nhiệm vụ**: 
   - Zeebe trả về các nhiệm vụ.
   - orenctl làm phong phú thêm thông tin về nhiệm vụ.
   - orenctl lưu trữ các nhiệm vụ trong bộ đệm.

3. **Thực hiện nhiệm vụ**:
   - orenctl worker định kỳ gửi yêu cầu gRPC đến orenctl để lấy nhiệm vụ.
   - orenctl gửi nhiệm vụ cho worker.
   - Worker thực hiện nhiệm vụ.

4. **Xử lý kết quả**:
   - Worker gửi kết quả đến orenctl (với cơ chế thử lại).
   - Nếu thành công, orenctl đánh dấu nhiệm vụ là đã hoàn thành trong Zeebe.
   - Nếu đạt đến số lần thử lại tối đa, worker sẽ bỏ qua kết quả.

5. **Xử lý nhiệm vụ hết hạn**: Các nhiệm vụ hết hạn có thể được lấy lại và thực hiện lại.

Sơ đồ này thể hiện rõ quá trình lặp lại và cơ chế xử lý lỗi trong hệ thống.

## 2. Sơ đồ Luồng: Quy trình của orenctl và orenctl worker

Sơ đồ luồng mô tả tổng quan về quy trình xử lý nhiệm vụ trong hệ thống.

### Các bước chính:

1. **Khởi đầu**: orenctl yêu cầu nhiệm vụ từ Zeebe.

2. **Xử lý ban đầu**:
   - Zeebe trả về nhiệm vụ.
   - orenctl làm phong phú và lưu trữ thông tin nhiệm vụ.

3. **Thực hiện nhiệm vụ**:
   - orenctl worker yêu cầu nhiệm vụ thông qua gRPC.
   - Worker thực hiện nhiệm vụ và gửi kết quả.

4. **Xử lý kết quả**:
   - Kiểm tra xem việc thử lại có thành công không.
   - Nếu thành công, đánh dấu nhiệm vụ hoàn thành trong Zeebe.
   - Nếu không, kiểm tra số lần thử lại.

5. **Xử lý lỗi**:
   - Nếu đạt đến số lần thử lại tối đa, bỏ qua kết quả.
   - Chờ đợi hết thời gian chờ của nhiệm vụ.
   - Quay lại bước đầu tiên để lấy lại nhiệm vụ.

Sơ đồ này thể hiện rõ các điểm quyết định trong quá trình và cách hệ thống xử lý các trường hợp lỗi hoặc hết thời gian chờ.

### Ý nghĩa chung của cả hai sơ đồ:

1. Thể hiện cơ chế retry và xử lý lỗi mạnh mẽ.
2. Minh họa quá trình xử lý nhiệm vụ liên tục và có khả năng phục hồi.
3. Cho thấy sự tương tác giữa các thành phần khác nhau trong hệ thống.
4. Thể hiện cách hệ thống đảm bảo rằng các nhiệm vụ cuối cùng sẽ được hoàn thành, ngay cả khi gặp lỗi hoặc hết thời gian chờ.

Hai sơ đồ này cung cấp cái nhìn toàn diện về cách hệ thống hoạt động, từ góc độ tuần tự thời gian (sơ đồ tuần tự) và góc độ logic quy trình (sơ đồ luồng).
