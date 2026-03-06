## How to Deploy the Controller

### Configurando `terraform/terraform.tfvars`

Preencha os valores abaixo e salve em `./terraform/terraform.tfvars`.

> Os valores abaixo são fictícios.

```tfvars
# ID do seu projeto no Google Cloud project_id = "you-project-id"

# O nome EXATO do bucket que você já criou manualmente ou via script anterior
existing_bucket_name = "bucket-name"

# Opcional (pois definimos default "us-central1" no variables.tf)
region = "us-central1"

admin_users = ["fams@linuxplace.io","franklin@linuxplace.io",
               "fabio@linuxplace.io","rodolfo@linuxplace.io"]

ado_org_url = "https://dev.azure.com/organization_name"

ado_pipeline_id = 10

ado_project = "Azure-Project-Name"

target_project_ids =  []

organization_domain = "gcp_organization_name.com.br"
```

### Configurando config.go

```
`./internal/admin/config/config.go`

alterar as urls do BACKEND (linha 56 e 60) para a URL do cloud run 

```
https://iac-controller-<xxxxxxxxxxx>.us-central1.run.app
```


### Confiurando o Container Registry

O script `./scripts/04-deploy-cloudrun.sh` faz o `docker build` e utiliza as
informações do `tfvars` criado anteriormente para criar o Container Registry e
configurar o Cloud Run para a aplicação *IAC Controller*.

Para isso, configure no seu terminal para que comandos `gcloud` esteja
apontados para o perfil correto. É possível fazer isso usando alguns dos
comandos:

- `gcloud config configurations list`
- `gcloud config list`
- `gcloud config confiurations create <NAME>`
- `gcloud auth login`
- `gcloud config set project <project_id>`
- `gcloud config set acccount <email>`
- `rm ~/.config/gcloud/application_default_credentials.json; \
     gcloud auth application-default login`

Após o `gcloud` configurado execute o `./scripts/04-deploy-cloudrun.sh` como
abaixo

```bash
bash ./scripts/04-deploy-cloudrun.sh
```

Caso tenha erros verifique permissões do seu usuário e rode novamente.

Quando o Cloud Run deve criar um serviço chamado `iac-controller` e começar a
instanciar o container. Valide nos logs o status de saúde e erros pois eles
indicam problemas a serem resolvidos como varáveis e arquivos de configurações
montados no container.

**ATENÇÃO:** alguns erros de variáveis e configs é esperado pois necessita do
próximo passo para serem solucionados.

### Configuração de Secrets

Nesse passo será necessário preencher, dentro do script
`./scripts/05-variables.sh` informações do `project_id` e do token `PAT`,
gerado pelo usuário principal da pipeline para acesso do GCP ao Azure DevOps
(ADO).

> O `PAT` se obtem dentro do do ADO, clicando no menu *User Settings ->
> Personal access token -> New Token -> Copie o token para um lugar seguro
> temporário*.

Com os dados obtidos, execute o script e verifique na console GCP se as secrets
foram criadas com sucesso.

```bash
bash ./scripts/05-variables.sh
```

### Recuperar o `repo_secret` do IAC Controller

Uma vez que o Cloud Run estiver em execução e sem erros, identifique a sua URL, executando os comandos abaixo, informando o client id e secret que devem ser criados no console da `GCP -> APi & Services -> Credentials`; e o url do controller criado no cloudrun.

Para executar o comando abaixo é necessario a instalação do golang-go versão 1.24.11

```bash
go build iac-admin ./cmd/admin/main.go
go build -o  iac-admin ./cmd/admin/main.go
  ./iac-admin init
  ./iac-admin register-repo "https://<USER>@dev.azure.com/<ORGANIZATION>/<PROJECT>/_git/<REPOSITORY NAME>"
```

### Instalação e Configuração do KEDA

> Para esse passo é necessário um cluster GKE em funcionamento com [Workload
  Identity Federation](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
  configurado.

#### Configure o KEDA

> Atenção! Verifique você tem permissões para alterações dentro do cluster
> antes de continuar! Também é necessário o binário Helm para este passo.

Instale o KEDA usando [documentação oficial
latest](https://keda.sh/docs/2.18/deploy/).

```bash
helm repo add kedacore https://kedacore.github.io/charts  
helm repo update

helm install keda kedacore/keda --namespace keda --create-namespace
kubectl get pods -n keda
```

#### Configure o Service Account com o Workload Identity Federation

Utilizando a documentação oficial do [Workload
Identity](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#authenticating_to),
configure o service account e associe ao IAM.

```bash
# Crie o novo ns do Azure Pipelines
kubectl create namespace azp

# Crie a service account dentro do Kubernetes
kubectl create serviceaccount controller-invoker \
    --namespace azp

# Associe a Service account ao IAM
#  Troque os valores de PROJECT_ID e PROJECT_NUMBER abaixo antes de executar
gcloud iam service-accounts add-iam-policy-binding
controller-invoker@prj-b-cicd-f60d.iam.gserviceaccount.com --role \
roles/iam.workloadIdentityUser --member \
"serviceAccount:prj-b-cicd-f60d.svc.id.goog[azp/azp-keda]"
```

#### Configure a Secret o KEDA

Configure a secret abaixo, com os respectivos valores corretos de `PAT` e
`URL`, obtidos anteriormente, e publique.

```bash
kubectl apply -f - <<EOF
apiVersion: v1
data:
  AZP_PAT: <path_em_base64>
  AZP_URL: <url_em_base64>
kind: Secret
metadata:
  name: azp-keda
  namespace: azp
type: Opaque
EOF
```

#### Configure o Trigger

Aplique o arquivo da forma abaixo, sem alterações:

```bash
kubectl apply -f - <<EOF
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata:
  name: azp-trigger-auth
  namespace: azp
spec:
  secretTargetRef:
  - key: AZP_PAT
    name: azp-keda
    parameter: personalAccessToken
EOF
```

### Finalizando e Testando o Keda

Para testar o Keda, é necessário que, além da execução desse documento, seja
também publicado a imagem que está configurada no repositório `lp-iac` chamada
`iac-cli`. Realize a configuração abaixo sabendo que está faltando a imagem do
`iac-cli` pois essa informação depende do repositório `lp-iac` na qual você
usará ao final desse documento.

#### Crie o ScaledJob do Agent

Altere os valores abaixo antes de aplicar:

`organizationURL`
`image`
`CONTROLLER_URL`
`poolID`
`COMPLIANCE_REGISTRY_URI`
`MODULE_REGISTRY_ROOT`

Certifique que a `serviceAccount` está igual a criada anteriormente e aplique.

> **ATENÇÃO:** para que esse serviço funcione, é necessário que a imagem
> `iac-cli` que está dentro do repositório `lp-iac` também esteja publicada.
> Aplique essa config abaixo com uma imagem inválida, vá até o repositório e
> realize sua publicação e atualize o deploy para continuar o teste.

```bash
kubectl apply -f - <<EOF
apiVersion: keda.sh/v1alpha1
kind: ScaledJob
metadata:
  name: azp-agent-plan
  namespace: azp
spec:
  failedJobsHistoryLimit: 3
  jobTargetRef:
    backoffLimit: 0
    template:
      metadata: {}
      spec:
        serviceAccount: controller-invoker
        containers:
        - env:
          - name: AZP_URL
            valueFrom:
              secretKeyRef:
                key: AZP_URL
                name: azp-keda
          - name: AZP_TOKEN
            valueFrom:
              secretKeyRef:
                key: AZP_PAT
                name: azp-keda
          - name: AZP_POOL
            value: gke-plan
          - name: TERRAFORM_BIN
            value: /usr/local/bin/terraform
          - name: TERRAGRUNT_BIN
            value: /usr/local/bin/terragrunt
          - name: AZP_AGENT_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: TF_PLUGIN_CACHE_DIR
            value: /tmp/terragrunt.d/modules
          - name: BASE_BRANCH
            value: origin/master
          - name: LOG_LEVEL
            value: debug
          - name: CONTROLLER_URL
            value: https://iac-controller-2xxxxxx96.us-central1.run.app
          - name: FEATURES_SECURITY_SCAN
            value: "true"
          - name: FEATURES_SECURITY_SCANNER
            value: trivy
          - name: FEATURES_TRIVY_BIN
            value: /usr/bin/trivy
          - name: FEATURES_PLAN_REGISTRATION
            value: "true"
          - name: COMPLIANCE_REGISTRY_URI
            value: gs://<PROJECT_ID>-pipeline-assets/blueprints/latest.json
          - name: MODULE_REGISTRY_ROOT
            value: https://dev.azure.com/<ORGANIZATION>/<PROJECT_NAME>/_git/<BLUEPRINT_REPO_NAME>
          image: us-central1-docker.pkg.dev/<PROJECT_ID>/iac-controller/iac-cli:<TAG_ID>
          imagePullPolicy: Always
          name: agent
          resources:
            requests:
              cpu: 250m
              memory: 512Mi
        imagePullSecrets:
        - name: gcp-auth
        restartPolicy: Never
        serviceAccount: controller-invoker
  rollout: {}
  scalingStrategy: {}
  triggers:
  - authenticationRef:
      name: azp-trigger-auth
    metadata:
      organizationURL: https://dev.azure.com/<organization_id>
      poolID: "<pool_id_number>"
      targetPipelinesQueueLength: "1"
    type: azure-pipelines
EOF
```



