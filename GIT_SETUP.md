# Configuración de Git Remote (GitLab)

El repositorio local ya está inicializado y el primer commit está hecho. Para hacer push a tu GitLab self-hosted, sigue estos pasos:

## 1. Crear el proyecto en GitLab

1. Ve a tu GitLab self-hosted
2. Crea un nuevo proyecto llamado `jarvis-dev`
3. Copia la URL del repositorio (SSH o HTTPS)

## 2. Configurar el remote

**Opción A: SSH** (recomendado si ya tienes SSH key configurada):
```bash
git remote add origin git@tu-gitlab.com:tu-usuario/jarvis-dev.git
```

**Opción B: HTTPS**:
```bash
git remote add origin https://tu-gitlab.com/tu-usuario/jarvis-dev.git
```

## 3. Push inicial

```bash
git push -u origin master
```

O si prefieres usar `main` como rama principal:
```bash
git branch -m master main
git push -u origin main
```

## 4. Verificar

```bash
git remote -v
# Deberías ver:
# origin  git@tu-gitlab.com:tu-usuario/jarvis-dev.git (fetch)
# origin  git@tu-gitlab.com:tu-usuario/jarvis-dev.git (push)
```

---

## Contenido del Commit Inicial

✅ **README.md**: Resumen ejecutivo del proyecto  
✅ **docs/ANALYSIS.md**: Análisis completo con todos los requerimientos  
✅ **docs/memories/**: 7 memorias exportadas del análisis  
✅ **.gitignore**: Archivos a ignorar

---

## IMPORTANTE: Regla para Futuros Commits

**Cada vez que hagas push a git, DEBES incluir las memorias actualizadas del proyecto.**

Para exportar memorias antes de cada commit, usa este workflow:

1. La IA actualiza las memorias automáticamente durante el trabajo
2. Antes de hacer push, ejecuta (cuando tengas el CLI listo):
   ```bash
   jarvis mem export docs/memories/
   ```
3. Commit + push:
   ```bash
   git add docs/memories/
   git commit -m "update: memorias del proyecto actualizadas"
   git push
   ```

**Esto garantiza que tengas TODO el contexto disponible en cualquier PC.**

---

Una vez configurado el remote, elimina este archivo:
```bash
rm GIT_SETUP.md
git add GIT_SETUP.md
git commit -m "chore: remove setup instructions"
git push
```
