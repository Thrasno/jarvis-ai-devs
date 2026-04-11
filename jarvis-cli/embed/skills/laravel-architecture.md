# Laravel Architecture — Conventions

## Core Principle: Thin Controllers

Controllers handle HTTP only. Business logic belongs in Services. Data access belongs in Repositories.

```
Request → Controller → Service → Repository → Model
                     ↓
                Response
```

## Controller Rules

```php
// GOOD — thin controller
class OrderController extends Controller
{
    public function store(StoreOrderRequest $request, OrderService $service): JsonResponse
    {
        $order = $service->createOrder($request->validated());
        return response()->json(new OrderResource($order), 201);
    }
}

// BAD — fat controller with business logic
class OrderController extends Controller
{
    public function store(Request $request): JsonResponse
    {
        // validation inline ❌
        $data = $request->validate([...]);
        // business logic inline ❌
        $order = Order::create($data);
        // email sending inline ❌
        Mail::to($order->customer)->send(new OrderConfirmation($order));
        return response()->json($order);
    }
}
```

## FormRequest Validation (Always)

Never validate in controllers. Always use FormRequest:

```php
class StoreOrderRequest extends FormRequest
{
    public function authorize(): bool
    {
        return $this->user()->can('create', Order::class);
    }

    public function rules(): array
    {
        return [
            'customer_id' => ['required', 'exists:customers,id'],
            'items'        => ['required', 'array', 'min:1'],
            'items.*.id'   => ['required', 'exists:products,id'],
        ];
    }
}
```

## Service Layer

Services contain business logic. They are injectable and testable:

```php
class OrderService
{
    public function __construct(
        private OrderRepository $orders,
        private InventoryService $inventory,
    ) {}

    public function createOrder(array $data): Order
    {
        // business logic here
        $this->inventory->reserve($data['items']);
        return $this->orders->create($data);
    }
}
```

## Jobs for Async Operations

Never block HTTP requests with slow operations:

```php
// In controller/service — dispatch, don't run inline
SendOrderConfirmationEmail::dispatch($order);
UpdateInventoryReport::dispatch($order)->delay(now()->addMinutes(5));
```

## Repository Pattern

```php
interface OrderRepositoryInterface
{
    public function create(array $data): Order;
    public function findByCustomer(int $customerId): Collection;
}

class EloquentOrderRepository implements OrderRepositoryInterface
{
    public function create(array $data): Order
    {
        return Order::create($data);
    }
}
```

Bind in AppServiceProvider:
```php
$this->app->bind(OrderRepositoryInterface::class, EloquentOrderRepository::class);
```
