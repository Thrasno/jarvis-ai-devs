# PHPUnit Testing — Patterns and Standards

## Arrange-Act-Assert (AAA) Structure

Every test follows three clearly separated sections:

```php
public function test_order_is_created_with_correct_total(): void
{
    // Arrange
    $customer = Customer::factory()->create();
    $product  = Product::factory()->create(['price' => 100]);

    // Act
    $order = $this->orderService->createOrder([
        'customer_id' => $customer->id,
        'items'       => [['id' => $product->id, 'quantity' => 2]],
    ]);

    // Assert
    $this->assertEquals(200, $order->total);
    $this->assertDatabaseHas('orders', ['customer_id' => $customer->id]);
}
```

## One Concept Per Test

Each test asserts ONE thing. Bad: testing multiple unrelated behaviors in one test.

```php
// BAD — multiple concepts
public function test_order_creation(): void
{
    $order = $this->service->createOrder([...]);
    $this->assertNotNull($order);            // concept 1
    $this->assertEquals(200, $order->total); // concept 2
    $this->assertCount(1, $order->items);    // concept 3
    $this->assertTrue($order->isConfirmed()); // concept 4
}

// GOOD — one concept per test
public function test_order_calculates_correct_total(): void { ... }
public function test_order_contains_submitted_items(): void { ... }
public function test_order_starts_in_pending_status(): void { ... }
```

## Factories Always

Never create raw model data in tests. Use factories:

```php
// BAD
$user = User::create(['name' => 'Test', 'email' => 'test@test.com', 'password' => bcrypt('pass')]);

// GOOD
$user = User::factory()->create();
$user = User::factory()->admin()->create(['email' => 'cto@company.com']);
```

## Test Naming Convention

`test_{what}_{condition}_{expected_outcome}`:
- `test_login_with_valid_credentials_returns_token`
- `test_login_with_wrong_password_returns_401`
- `test_order_without_items_throws_validation_exception`

## Mocking External Services

Always mock external APIs and services:

```php
public function test_sends_confirmation_email_after_order(): void
{
    Mail::fake();

    $order = $this->service->createOrder([...]);

    Mail::assertSent(OrderConfirmation::class, function ($mail) use ($order) {
        return $mail->hasTo($order->customer->email);
    });
}
```

## Database Tests

Use `RefreshDatabase` or `DatabaseTransactions`:

```php
class OrderTest extends TestCase
{
    use RefreshDatabase; // rolls back after each test

    // or for speed:
    use DatabaseTransactions; // uses transactions instead of truncate
}
```
