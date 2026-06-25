# Async Engine — State Machine

## Visão Geral do Fluxo Completo

```mermaid
sequenceDiagram
    participant R as Runner (iac-runner)
    participant C as Controller (iac-controller)
    participant F as Firestore
    participant CT as Cloud Tasks
    participant SCM as Azure DevOps

    Note over R,SCM: PLAN FLOW
    R->>C: AccessToken(mode=plan)
    C-->>R: GCP token (plan SA)
    R->>R: terragrunt plan
    alt queue não vazia
        R->>C: RegisterPlan(SHA, stacks, output)
        C->>F: SaveDeployment(SourceBranchSHA)
    else queue vazia (sem mudanças terraform)
        R->>C: RegisterPlan(stacks=[], SHA)
        C->>F: SaveDeployment(SourceBranchSHA)
    end

    Note over R,SCM: APPLY FLOW
    R->>C: AccessToken(mode=apply, SourceBranchSHA)
    C->>SCM: GetPullRequest
    C->>SCM: BranchStatus
    C->>F: GetDeploymentByPR
    C->>SCM: GetPRPolicyStatus
    C->>C: ApplyEngine.Check() [sha_stale, branch_up_to_date, sha_backend_match, branch_policies_passing]
    C->>F: AcquireBatch (locks)
    C->>F: SaveDeployment(SourceBranchSHA=req.SHA)
    C-->>R: GCP token (apply SA)
    R->>R: terragrunt apply
    R->>C: ClosePlan
    C->>F: ReleaseBatch (locks)
    C->>F: Kick(merge-pr, deployment.ID)
    C->>CT: EnqueueRun(delay=5s)

    Note over R,SCM: MERGE FLOW (async)
    CT->>C: POST /internal/async/run
    C->>F: AcquireLease
    C->>SCM: MergePR(SourceBranchSHA)
    alt merge ok
        C->>F: MarkDone
    else falha permanente (TF401192, 403, etc.)
        C->>SCM: SetStatus(failure) + CommentUpdate
        C->>F: MarkFailed
    else falha transitória (5xx)
        C->>F: MarkWaiting(60s)
        C->>CT: EnqueueRun(delay=60s)
    end
```

---

## Diagrama de Estados

```mermaid
stateDiagram-v2
    [*] --> queued : Kick() — novo documento\nou done/failed/invalid

    queued --> queued : Kick() — fresco (<10min)\ncoalesce
    queued --> queued : Kick() — stale (>10min) ou wakeNow=true\nre-enfileira
    queued --> running : AcquireLease()\nattempt+1

    running --> running : Kick() — lease ativo\ndirty=true, coalesce
    running --> queued  : Kick() — lease expirado\nattempt=0, re-enfileira
    running --> done    : OutcomeDone → MarkDone()\ndirty=false
    running --> queued  : OutcomeDone → MarkDone()\ndirty=true → requeue imediato
    running --> waiting : OutcomeWait → MarkWaiting()\ndirty=false → EnqueueRun(delay)
    running --> queued  : OutcomeWait → MarkWaiting()\ndirty=true → EnqueueRun(0)
    running --> failed  : OutcomeFail → MarkFailed()\ndirty=false
    running --> queued  : OutcomeFail → MarkFailed()\ndirty=true → requeue imediato
    running --> running : OutcomeRetry → ClearLease()\nHTTP 500 → CT re-entrega

    waiting --> queued  : Kick(wakeNow=true)\nwake_at limpo
    waiting --> waiting : Kick(wakeNow=false)\ncoalesce
    waiting --> running : AcquireLease() — wake_at <= now

    done --> queued : Kick()\nattempt=0, re-enfileira
    failed --> queued : Kick()\nattempt=0, re-enfileira
```

---

## Arquitetura do Engine

### Invariantes

1. **Dirty check atômico**: cada `Mark*` checa o flag `dirty` na mesma transação Firestore que escreve o novo estado. Não há janela de race entre a transição e a verificação.

2. **Retry preserva o lease limpo**: `OutcomeRetry` chama `ClearLease` antes de retornar HTTP 500, permitindo que Cloud Tasks re-entregue imediatamente sem esperar o TTL expirar.

3. **Estados terminais são idempotentes**: `AcquireLease` rejeita `done`/`failed` — re-entregas tardias do Cloud Tasks são no-ops.

4. **wait_at é respeitado**: `AcquireLease` rejeita aquisição se `status=waiting` e `wake_at > now`.

5. **OutcomeWait com dirty não desperdiça mensagem**: se dirty=true durante `MarkWaiting`, o engine enfileira com `delay=0` (sem enfileirar o delay original).

### Interface Store (7 métodos)

```
UpsertQueued    — cria ou atualiza para queued; retorna se deve enfileirar
AcquireLease    — adquire lease atômico; recusa terminais e waiting precoce
Get             — leitura simples para observabilidade
MarkDone        — transição done (ou queued se dirty); retorna requeue bool
MarkWaiting     — transição waiting (ou queued se dirty); retorna requeue bool
MarkFailed      — transição failed (ou queued se dirty); retorna requeue bool
ClearLease      — zera lease sem alterar status (usado por OutcomeRetry)
```

### Fluxo de RunOnce

```
AcquireLease → not acquired → HTTP 200 (no-op)
             → acquired
                 ↓
             registry.Get → not found → MarkFailed("no handler") → HTTP 200
                          → found
                              ↓
                          h.Run(ctx, exec) → runErr → HTTP 500
                                           → outcome
                                               ↓
                          OutcomeRetry → ClearLease → HTTP 500
                          OutcomeDone  → MarkDone(checkpoint)  → requeue? EnqueueRun(0)
                          OutcomeWait  → MarkWaiting(wakeAt)   → requeue? EnqueueRun(0)
                                                                        : EnqueueRun(delay)
                          OutcomeFail  → MarkFailed(err)       → requeue? EnqueueRun(0)
                          default      → MarkFailed("unknown") → requeue? EnqueueRun(0)
                              ↓
                          HTTP 200
```

---

## Problemas conhecidos abertos

### P6 — Locks não são liberados quando apply falha no runner

**Arquivo:** `iac-runner-private`

Se o `terragrunt apply` falhar, `ClosePlan` não é chamado (é semântico de sucesso). Os locks de stack (`AcquireBatch`) ficam presos até o PR ser fechado ou `/unlock` ser executado manualmente.

**Nota:** Locks **nunca devem ser liberados automaticamente** — isso é uma garantia de segurança intencional para evitar apply concorrente no mesmo stack.

**Mitigação atual:** Operador executa `/unlock` no PR ou fecha e reabre o PR.
