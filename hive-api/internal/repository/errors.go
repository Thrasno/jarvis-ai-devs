package repository

import "errors"

// ErrNotFound se devuelve cuando una entidad no existe en la base de datos.
//
// En Go los errores centinela son variables globales que representan
// condiciones específicas. El llamador puede comparar con errors.Is():
//
//   user, err := repo.GetByEmail(ctx, email)
//   if errors.Is(err, repository.ErrNotFound) {
//       // el usuario no existe → devolver 404
//   }
//
// Es el equivalente a una excepción tipada en PHP:
//   catch (NotFoundException $e) { ... }
// pero sin el overhead de excepciones.
var ErrNotFound = errors.New("not found")

// ErrConflict se devuelve cuando una operación viola una restricción única
// (por ejemplo, intentar crear un usuario con un email que ya existe).
// El handler lo mapea a un HTTP 409 Conflict.
var ErrConflict = errors.New("conflict")
