class CommerceError(Exception):
    code = "INTERNAL"
    status = 500

    def __init__(self, message: str):
        super().__init__(message)
        self.message = message


class InvalidInput(CommerceError):
    code = "INVALID_INPUT"
    status = 400


class InsufficientInventory(CommerceError):
    code = "INSUFFICIENT_INVENTORY"
    status = 409


class IdempotencyConflict(CommerceError):
    code = "IDEMPOTENCY_CONFLICT"
    status = 409


class NotFound(CommerceError):
    code = "NOT_FOUND"
    status = 404


class InvalidTransition(CommerceError):
    code = "INVALID_TRANSITION"
    status = 409


class CurrencyMismatch(CommerceError):
    code = "CURRENCY_MISMATCH"
    status = 400
